/**
 * Base64 payload cap for one transcription request (~12 MB of audio).
 * Keeps a long mono 16 kHz WAV dictation (≈6 minutes) under the limit
 * while bounding what the renderer can push over IPC.
 */
export const SPEECH_TRANSCRIPTION_MAX_BASE64_CHARS = 16_000_000

/** Hard cap on a single dictation so the payload stays under the IPC limit. */
export const SPEECH_TRANSCRIPTION_MAX_DURATION_MS = 5 * 60 * 1000

export type SpeechTranscriptionRequest = {
  /** Base64-encoded audio bytes (no data: prefix). */
  audioBase64: string
  /** Audio MIME type, e.g. "audio/wav". */
  mimeType: string
  /** Optional recording duration, for logging/limits. */
  durationMs?: number
}

export type SpeechTranscriptionResult =
  | {
      ok: true
      text: string
    }
  | {
      ok: false
      message: string
    }
