import { beforeEach, describe, expect, it } from 'vitest'
import i18n from '../i18n'
import { describeRuntimeError, formatRuntimeError, getRuntimeErrorCode } from './format-runtime-error'
import { parseRuntimeErrorBody, runtimeErrorToError } from '@shared/runtime-error'

describe('format runtime error', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en')
  })

  it('uses code fields for localized summaries and settings actions', () => {
    const error = new Error(JSON.stringify({
      code: 'missing_api_key',
      message: 'api-key=sk-test is missing',
      details: { Authorization: 'Bearer runtime-token' }
    }))

    const view = describeRuntimeError(error)

    expect(view.summary).toBe(i18n.t('common:runtimeMissingApiKey'))
    expect(view.summary).not.toContain('DeepSeek')
    expect(view.code).toBe('missing_api_key')
    expect(view.settingsAction).toBe('agents')
    expect(view.detail).not.toContain('Authorization')
    expect(view.detail).not.toContain('sk-test')
    expect(view.detail).not.toContain('runtime-token')
  })

  it('supports legacy error envelopes and Electron IPC prefixes', () => {
    const error = new Error(
      `Error invoking remote method 'runtime:request': Error: ${JSON.stringify({
        error: 'fetch_failed',
        message: 'fetch failed'
      })}`
    )

    expect(getRuntimeErrorCode(error)).toBe('fetch_failed')
    expect(formatRuntimeError(error)).toBe(i18n.t('common:runtimeFetchFailed'))
  })

  it('summarizes structured runtime availability diagnostics without leaking tokens', () => {
    const error = new Error(JSON.stringify({
      code: 'runtime_unavailable',
      message: 'failed to reach Analytix runtime at http://127.0.0.1:1/v1/threads?token=query-secret: fetch failed',
      severity: 'error',
      details: {
        baseUrl: 'http://127.0.0.1:1',
        path: '/v1/threads?token=query-secret',
        port: 1,
        backend: 'go-runtime-default',
        requestedBackend: '',
        cause: 'connect ECONNREFUSED token=query-secret'
      }
    }))

    const view = describeRuntimeError(error)

    expect(view.summary).toBe(i18n.t('common:runtimeFetchFailed'))
    expect(view.code).toBe('runtime_unavailable')
    expect(view.settingsAction).toBe('agents')
    expect(view.detail).not.toContain('baseUrl')
    expect(view.detail).toContain('go-runtime-default')
    expect(view.detail).not.toContain('cause')
    expect(view.detail).not.toContain('query-secret')
  })

  it('never exposes raw provider messages for unknown errors', () => {
    const message = `model request failed with status 418: ${JSON.stringify({
      error: {
        code: '418',
        message: `Not supported model ${'mimo-v2.5-pro-ultraspeed'.repeat(20)}`
      }
    })}`
    const error = new Error(JSON.stringify({
      code: 'http_418',
      message,
      severity: 'error'
    }))

    const view = describeRuntimeError(error)

    expect(view.summary).toBe(i18n.t('common:runtimeRequestFailed'))
    expect(view.detail).toContain('Code: http_418')
    expect(view.detail).toContain('Severity: error')
    expect(view.detail).not.toContain(message)
    expect(JSON.stringify(view)).not.toContain('mimo-v2.5-pro-ultraspeed')
  })

  it('summarizes provider 404s with configuration guidance and redacted diagnostics', () => {
    const error = new Error(JSON.stringify({
      code: 'http_404',
      message: 'model request failed with status 404: upstream token=message-secret',
      severity: 'error',
      details: {
        provider: 'deepseek',
        providerId: 'deepseek',
        model: 'deepseek-chat',
        endpointFormat: 'chat_completions',
        baseUrl: 'https://api.example.com/v1?api_key=url-secret',
        requestUrl: 'https://api.example.com/v1/chat/completions?token=request-secret',
        status: 404,
        responseBody: 'x-api-key: sk-responseSecretValue1234567890'
      }
    }))

    const view = describeRuntimeError(error)

    expect(view.summary).toBe(i18n.t('common:runtimeProviderConfigurationError'))
    expect(view.settingsAction).toBe('agents')
    expect(view.detail).toContain('Code: http_404')
    expect(view.detail).not.toContain('baseUrl')
    expect(view.detail).not.toContain('requestUrl')
    expect(view.detail).toContain('endpointFormat')
    expect(view.detail).toContain('deepseek-chat')
    expect(view.detail).toContain(i18n.t('common:runtimeProviderConfigurationHint'))
    expect(view.detail).not.toContain('responseBody')
    expect(view.detail).not.toContain('message-secret')
    expect(view.detail).not.toContain('url-secret')
    expect(view.detail).not.toContain('request-secret')
    expect(view.detail).not.toContain('sk-responseSecretValue1234567890')
  })

  it('surfaces top-level providerError diagnostics from Go runtime turn failures', () => {
    const error = new Error(JSON.stringify({
      code: 'turn_failed',
      message: 'provider auth-provider returned 401 requestUrl=https://api.example.com/v1/chat/completions?token=request-secret: x-api-key: sk-messageSecret1234567890',
      severity: 'error',
      providerError: {
        providerId: 'auth-provider',
        family: 'openai-compatible',
        endpointFormat: 'chat_completions',
        baseUrl: 'https://api.example.com/v1?api_key=url-secret',
        requestUrl: 'https://api.example.com/v1/chat/completions?token=request-secret',
        status: 401,
        kind: 'auth',
        message: 'x-api-key: sk-providerSecret1234567890 rejected',
        hasApiKey: true,
        authStatus: 'required',
        retryable: false
      }
    }))

    const view = describeRuntimeError(error)

    expect(view.summary).toBe(i18n.t('common:runtimeProviderConfigurationError'))
    expect(view.code).toBe('turn_failed')
    expect(view.settingsAction).toBe('agents')
    expect(view.detail).toContain('Provider diagnostic')
    expect(view.detail).toContain('auth-provider')
    expect(view.detail).toContain('endpointFormat')
    expect(view.detail).toContain('chat_completions')
    expect(view.detail).toContain('hasApiKey')
    expect(view.detail).toContain(i18n.t('common:runtimeProviderConfigurationHint'))
    expect(view.detail).not.toContain('baseUrl')
    expect(view.detail).not.toContain('request-secret')
    expect(view.detail).not.toContain('url-secret')
    expect(view.detail).not.toContain('sk-messageSecret1234567890')
    expect(view.detail).not.toContain('sk-providerSecret1234567890')
  })

  it('surfaces provider insufficient balance diagnostics as an actionable provider issue', () => {
    const error = new Error(JSON.stringify({
      code: 'turn_failed',
      message: 'provider deepseek returned 402 requestUrl=https://api.example.com/v1/chat/completions?token=request-secret: api_key=sk-messageSecret1234567890 insufficient balance',
      severity: 'error',
      providerError: {
        providerId: 'deepseek',
        family: 'deepseek',
        endpointFormat: 'chat_completions',
        baseUrl: 'https://api.example.com/v1?api_key=url-secret',
        requestUrl: 'https://api.example.com/v1/chat/completions?token=request-secret',
        status: 402,
        kind: 'insufficient_balance',
        message: 'api_key=sk-providerSecret1234567890 insufficient balance',
        hasApiKey: true,
        authStatus: 'none',
        retryable: false
      }
    }))

    const view = describeRuntimeError(error)

    expect(view.summary).toBe(i18n.t('common:runtimeProviderBillingError'))
    expect(view.code).toBe('turn_failed')
    expect(view.settingsAction).toBe('agents')
    expect(view.detail).toContain('Provider diagnostic')
    expect(view.detail).toContain('insufficient_balance')
    expect(view.detail).toContain(i18n.t('common:runtimeProviderBillingHint'))
    expect(view.detail).not.toContain('baseUrl')
    expect(view.detail).not.toContain('request-secret')
    expect(view.detail).not.toContain('url-secret')
    expect(view.detail).not.toContain('sk-messageSecret1234567890')
    expect(view.detail).not.toContain('sk-providerSecret1234567890')
  })

  it('uses a host reason code while dropping provider diagnostics at the HTTP boundary', () => {
    const parsed = parseRuntimeErrorBody(JSON.stringify({
      code: 'turn_failed',
      reasonCode: 'provider_authentication_failed',
      message: 'provider auth-provider returned 401 requestUrl=https://api.example.com/v1/chat/completions?token=request-secret',
      providerError: {
        providerId: 'auth-provider',
        endpointFormat: 'chat_completions',
        status: 401,
        kind: 'auth',
        hasApiKey: true,
        authStatus: 'required'
      }
    }), 'fallback')
    const view = describeRuntimeError(runtimeErrorToError(parsed))

    expect(view.summary).toBe(i18n.t('common:runtimeProviderConfigurationError'))
    expect(view.code).toBe('turn_failed')
    expect(view.settingsAction).toBe('agents')
    expect(view.detail).not.toContain('Provider diagnostic')
    expect(view.detail).not.toContain('auth-provider')
    expect(view.detail).not.toContain('hasApiKey')
    expect(view.detail).not.toContain('request-secret')
  })

  it('reads provider diagnostics nested under details for retry and pipeline errors', () => {
    const error = new Error(JSON.stringify({
      code: 'turn_failed',
      message: 'provider failed during retry requestUrl=https://api.example.com/v1/messages?token=request-secret',
      details: {
        attempt: 2,
        providerError: {
          providerId: 'anthropic-compatible',
          endpointFormat: 'messages',
          status: 404,
          kind: 'http_status',
          hasApiKey: true
        }
      }
    }))

    const view = describeRuntimeError(error)

    expect(view.summary).toBe(i18n.t('common:runtimeProviderConfigurationError'))
    expect(view.code).toBe('turn_failed')
    expect(view.settingsAction).toBe('agents')
    expect(view.detail).toContain('Details')
    expect(view.detail).toContain('Provider diagnostic')
    expect(view.detail).toContain('anthropic-compatible')
    expect(view.detail).toContain('messages')
    expect(view.detail).toContain(i18n.t('common:runtimeProviderConfigurationHint'))
    expect(view.detail).not.toContain('request-secret')
  })
})
