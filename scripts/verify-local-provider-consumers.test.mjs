import assert from 'node:assert/strict'
import test from 'node:test'

import {
  forbiddenMediaProviderCapabilities,
  forbiddenProviderEgressCapabilities,
  forbiddenRendererSpeechAuthorityCapabilities,
  forbiddenCLIProviderAuthorityArgs,
  isProviderLikeNetworkEffect
} from './verify-local-provider-consumers.mjs'

test('detects executable Provider egress capabilities instead of checking inventory names', () => {
  const capabilities = forbiddenProviderEgressCapabilities(`
    const apiKey = settings.provider.apiKey
    await fetch(settings.provider.baseUrl, {
      headers: { Authorization: 'Bearer ' + apiKey }
    })
  `)

  assert.deepEqual(capabilities, [
    'direct fetch',
    'credential field',
    'provider authorization header',
    'copied provider route'
  ])
})

test('accepts key-free intent sent to the Go runtime boundary', () => {
  assert.deepEqual(forbiddenProviderEgressCapabilities(`
    await runtimeRequest('/v1/threads/thread-1/turns', 'POST', JSON.stringify({
      prompt,
      model,
      reasoningEffort,
      disableUserInput: true
    }))
  `), [])
})

test('detects a preconstructed Provider SDK client without relying on fetch', () => {
  assert.deepEqual(forbiddenProviderEgressCapabilities(`
    import OpenAI from 'openai'
    const client = new OpenAI()
  `), ['provider SDK'])
})

test('detects newly introduced direct media Provider authority and egress', () => {
  assert.deepEqual(forbiddenMediaProviderCapabilities(`
    const protocol = settings.runtime.imageGeneration.protocol
    const baseUrl = settings.runtime.imageGeneration.baseUrl
    const apiKey = settings.runtime.imageGeneration.apiKey
    await fetch(baseUrl, { headers: { Authorization: 'Bearer ' + apiKey } })
  `), [
    'direct network',
    'provider credential',
    'provider route',
    'provider protocol'
  ])
})

test('accepts bounded media intent delegated to the private Go runtime', () => {
  assert.deepEqual(forbiddenMediaProviderCapabilities(`
    await executeMediaRequest({
      operation: 'speech.transcribe',
      audioBase64,
      mimeType,
      timeoutMs
    })
  `), [])
})

test('detects copied renderer speech settings that gate or enter an execution hook', () => {
  assert.deepEqual(forbiddenRendererSpeechAuthorityCapabilities(`
    const speechToTextSettings = resolveAnalytixSpeechToTextSettings(settings)
    const visible = speechToTextSettings.baseUrl && speechToTextSettings.model
    useVoiceDictation({ speechToText: speechToTextSettings })
  `), [
    'resolved Provider config',
    'Provider field gate',
    'copied config forwarding'
  ])

  assert.deepEqual(forbiddenRendererSpeechAuthorityCapabilities(`
    const enabled = settings.runtime.speechToText.enabled === true
    useVoiceDictation({ onText })
  `), [])
})

test('classifies a newly introduced media endpoint fetch as Provider egress', () => {
  assert.equal(isProviderLikeNetworkEffect(`
    await fetch(endpoint + '/v1/images/generations', {
      method: 'POST',
      body: JSON.stringify({ model, prompt })
    })
  `), true)
  assert.equal(isProviderLikeNetworkEffect(`
    await runtimeRequest('/v1/runtime/_private/media-execution', 'POST', body)
  `), false)
})

test('detects copied CLI Provider authority while allowing dataDir and runtime authentication', () => {
  assert.deepEqual(forbiddenCLIProviderAuthorityArgs(`
    return ['--data-dir', dataDir, '--base-url', copiedBaseUrl, '--endpoint-format', copiedFormat]
  `), ['--base-url', '--endpoint-format'])
  assert.deepEqual(forbiddenCLIProviderAuthorityArgs(`
    return ['--data-dir', dataDir, '--runtime-token-mode', 'environment']
  `), [])
})
