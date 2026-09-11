import { describe, expect, it } from 'vitest'
import type { AppSettingsV1 } from '../../shared/app-settings'
import {
  requestSpeechTranscription,
  type PrivateMediaSpeechRequest
} from './speech-to-text-service'

const AUDIO_BASE64 = Buffer.from('fake-wav-bytes').toString('base64')

function settingsWithSpeech(): AppSettingsV1 {
  return {
    runtime: {
      speechToText: {
        enabled: true,
        providerId: 'must-not-cross-main-provider',
        protocol: 'mimo-asr',
        baseUrl: 'https://must-not-cross-main.example/v1',
        model: 'must-not-cross-main-model',
        language: 'zh',
        timeoutMs: 30000
      }
    }
  } as unknown as AppSettingsV1
}

describe('speech-to-text service', () => {
  it('fails closed until the private Go media executor is available', async () => {
    const result = await requestSpeechTranscription(
      settingsWithSpeech(),
      { audioBase64: AUDIO_BASE64, mimeType: 'audio/wav' }
    )
    expect(result).toEqual({ ok: false, message: 'speech-to-text provider is not configured' })
  })

  it('delegates only bounded audio and key-free user intent', async () => {
    const requests: PrivateMediaSpeechRequest[] = []
    const result = await requestSpeechTranscription(
      settingsWithSpeech(),
      { audioBase64: AUDIO_BASE64, mimeType: 'audio/wav' },
      { executeMediaRequest: async (request) => {
        requests.push(request)
        return { ok: true, transcript: '  这是转写结果。  ' }
      } }
    )

    expect(requests).toEqual([{
      operation: 'speech.transcribe',
      audioBase64: AUDIO_BASE64,
      mimeType: 'audio/wav',
      language: 'zh',
      timeoutMs: 30000
    }])
    expect(JSON.stringify(requests)).not.toMatch(/apiKey|baseUrl|providerId|protocol|model|credentialRef|endpoint|proxy|Authorization|must-not-cross-main/i)
    expect(result).toEqual({ ok: true, text: '这是转写结果。' })
  })

  it('projects executor failures without raw Provider content', async () => {
    const sentinel = '/private/customer-13900000017 raw-provider-body'
    const result = await requestSpeechTranscription(
      settingsWithSpeech(),
      { audioBase64: AUDIO_BASE64, mimeType: 'audio/wav' },
      { executeMediaRequest: async () => { throw new Error(sentinel) } }
    )
    expect(result).toEqual({ ok: false, message: 'speech provider request failed' })
    expect(JSON.stringify(result)).not.toContain(sentinel)
  })
})
