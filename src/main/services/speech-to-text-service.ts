import type { AppSettingsV1 } from '../../shared/app-settings'
import {
  SPEECH_TRANSCRIPTION_MAX_BASE64_CHARS,
  type SpeechTranscriptionRequest,
  type SpeechTranscriptionResult
} from '../../shared/speech-to-text'

export type PrivateMediaSpeechRequest = {
  operation: 'speech.transcribe'
  audioBase64: string
  mimeType: string
  language?: string
  timeoutMs: number
}

export type PrivateMediaSpeechResult =
  | { ok: true; transcript: string }
  | { ok: false; code: 'invalid_request' | 'unavailable' | 'authority_changed' | 'provider_failed' }

export type ExecutePrivateMediaSpeechRequest = (
  request: PrivateMediaSpeechRequest
) => Promise<PrivateMediaSpeechResult>

function speechIntentFromSettings(settings: AppSettingsV1): { language: string; timeoutMs: number } {
  const candidate = (settings as {
    runtime?: { speechToText?: { language?: unknown; timeoutMs?: unknown } }
  }).runtime?.speechToText
  const language = typeof candidate?.language === 'string' && candidate.language.length <= 16
    ? candidate.language.trim()
    : ''
  const timeoutMs = typeof candidate?.timeoutMs === 'number' && Number.isInteger(candidate.timeoutMs) &&
    candidate.timeoutMs > 0 && candidate.timeoutMs <= 300_000
    ? candidate.timeoutMs
    : 30_000
  return { language, timeoutMs }
}

export async function requestSpeechTranscription(
  settings: AppSettingsV1,
  request: SpeechTranscriptionRequest,
  options: { executeMediaRequest?: ExecutePrivateMediaSpeechRequest } = {}
): Promise<SpeechTranscriptionResult> {
  if (!request.audioBase64 || request.audioBase64.length > SPEECH_TRANSCRIPTION_MAX_BASE64_CHARS) {
    return { ok: false, message: 'audio payload is empty or too large' }
  }
  if (!options.executeMediaRequest) {
    return { ok: false, message: 'speech-to-text provider is not configured' }
  }
  const intent = speechIntentFromSettings(settings)
  let result: PrivateMediaSpeechResult
  try {
    result = await options.executeMediaRequest({
      operation: 'speech.transcribe',
      audioBase64: request.audioBase64,
      mimeType: request.mimeType,
      ...(intent.language ? { language: intent.language } : {}),
      timeoutMs: intent.timeoutMs
    })
  } catch {
    return { ok: false, message: 'speech provider request failed' }
  }
  if (!result.ok) {
    return {
      ok: false,
      message: result.code === 'unavailable'
        ? 'speech-to-text provider is not configured'
        : result.code === 'authority_changed'
          ? 'speech provider changed during the request'
          : 'speech provider request failed'
    }
  }
  const transcript = result.transcript.trim()
  if (!transcript || transcript.length > 64 * 1024) {
    return { ok: false, message: 'transcription result is empty' }
  }
  return { ok: true, text: transcript }
}
