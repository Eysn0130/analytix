import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { AppSettingsV1 } from '@shared/app-settings'
import { SPEECH_TRANSCRIPTION_MAX_DURATION_MS } from '@shared/speech-to-text'
import { SETTINGS_CHANGED_EVENT } from '../../lib/keyboard-shortcut-settings'
import { rendererRuntimeClient } from '../../agent/runtime-client'

export type VoiceDictationStatus = 'idle' | 'recording' | 'transcribing'

/** What to do with the transcript once it lands: insert into the input, or send right away. */
export type VoiceDictationIntent = 'insert' | 'send'

const TRANSCRIPTION_SAMPLE_RATE = 16_000
const MIN_RECORDING_MS = 500
const RECORDER_MIME_CANDIDATES = ['audio/webm;codecs=opus', 'audio/webm', 'audio/mp4']

/** Key-free UI preference, kept in sync with the settings screen. */
export function speechToTextPreferenceEnabled(settings: Pick<AppSettingsV1, 'runtime'>): boolean {
  return settings.runtime.speechToText.enabled === true
}

export function useSpeechToTextEnabled(): boolean {
  const [enabled, setEnabled] = useState(false)

  useEffect(() => {
    let cancelled = false
    const apply = (settings: AppSettingsV1): void => {
      if (!cancelled) setEnabled(speechToTextPreferenceEnabled(settings))
    }
    void rendererRuntimeClient.getSettings().then(apply).catch(() => undefined)
    const onSettingsChanged = (event: Event): void => {
      apply((event as CustomEvent<AppSettingsV1>).detail)
    }
    window.addEventListener(SETTINGS_CHANGED_EVENT, onSettingsChanged)
    return () => {
      cancelled = true
      window.removeEventListener(SETTINGS_CHANGED_EVENT, onSettingsChanged)
    }
  }, [])

  return enabled
}

export function useVoiceDictation({
  onText
}: {
  onText: (text: string, intent: VoiceDictationIntent) => void
}): {
  status: VoiceDictationStatus
  error: string | null
  startedAtMs: number
  start: () => void
  stop: (intent?: VoiceDictationIntent) => void
  toggle: () => void
  /** Current microphone level (0..1) for waveform rendering. Safe to call every frame. */
  getLevel: () => number
} {
  const { t } = useTranslation('common')
  const [status, setStatus] = useState<VoiceDictationStatus>('idle')
  const [error, setError] = useState<string | null>(null)
  const [startedAtMs, setStartedAtMs] = useState(0)
  const recorderRef = useRef<MediaRecorder | null>(null)
  const streamRef = useRef<MediaStream | null>(null)
  const audioContextRef = useRef<AudioContext | null>(null)
  const analyserRef = useRef<AnalyserNode | null>(null)
  const levelDataRef = useRef<Uint8Array<ArrayBuffer> | null>(null)
  const stopIntentRef = useRef<VoiceDictationIntent>('insert')
  const maxDurationTimerRef = useRef<ReturnType<typeof window.setTimeout> | null>(null)
  const startedAtRef = useRef(0)
  const onTextRef = useRef(onText)
  const mountedRef = useRef(true)

  useEffect(() => {
    onTextRef.current = onText
  }, [onText])

  const releaseStream = useCallback((): void => {
    if (maxDurationTimerRef.current != null) {
      window.clearTimeout(maxDurationTimerRef.current)
      maxDurationTimerRef.current = null
    }
    streamRef.current?.getTracks().forEach((track) => track.stop())
    streamRef.current = null
    recorderRef.current = null
    analyserRef.current = null
    levelDataRef.current = null
    void audioContextRef.current?.close().catch(() => undefined)
    audioContextRef.current = null
  }, [])

  const getLevel = useCallback((): number => {
    const analyser = analyserRef.current
    const data = levelDataRef.current
    if (!analyser || !data) return 0
    analyser.getByteTimeDomainData(data)
    let sumSquares = 0
    for (let i = 0; i < data.length; i += 1) {
      const value = (data[i] - 128) / 128
      sumSquares += value * value
    }
    return Math.min(1, Math.sqrt(sumSquares / data.length) * 3)
  }, [])

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      recorderRef.current?.stop()
      releaseStream()
    }
  }, [releaseStream])

  const transcribeBlob = useCallback(async (blob: Blob, durationMs: number, intent: VoiceDictationIntent): Promise<void> => {
    try {
      const wav = await encodeBlobAsWav(blob)
      const result = await window.analytix.speech.transcribe({
        audioBase64: wav.base64,
        mimeType: 'audio/wav',
        durationMs: Math.min(durationMs, SPEECH_TRANSCRIPTION_MAX_DURATION_MS)
      })
      if (!mountedRef.current) return
      if (result.ok) {
        onTextRef.current(result.text, intent)
      } else {
        setError(t('composerVoiceFailed', { message: result.message }))
      }
    } catch (cause) {
      if (mountedRef.current) {
        setError(t('composerVoiceFailed', { message: cause instanceof Error ? cause.message : String(cause) }))
      }
    } finally {
      if (mountedRef.current) setStatus('idle')
    }
  }, [t])

  const start = useCallback((): void => {
    if (recorderRef.current) return
    setError(null)
    void (async () => {
      let stream: MediaStream
      try {
        stream = await navigator.mediaDevices.getUserMedia({ audio: true })
      } catch (cause) {
        const denied = cause instanceof DOMException &&
          (cause.name === 'NotAllowedError' || cause.name === 'SecurityError')
        if (mountedRef.current) {
          setError(denied
            ? t('composerVoiceMicDenied')
            : t('composerVoiceFailed', { message: cause instanceof Error ? cause.message : String(cause) }))
        }
        return
      }
      if (!mountedRef.current) {
        stream.getTracks().forEach((track) => track.stop())
        return
      }
      const mimeType = RECORDER_MIME_CANDIDATES.find((candidate) =>
        MediaRecorder.isTypeSupported(candidate)
      )
      const recorder = new MediaRecorder(stream, mimeType ? { mimeType } : undefined)
      const chunks: Blob[] = []
      recorder.ondataavailable = (event) => {
        if (event.data.size > 0) chunks.push(event.data)
      }
      recorder.onstop = () => {
        const durationMs = Date.now() - startedAtRef.current
        const intent = stopIntentRef.current
        const blob = new Blob(chunks, { type: recorder.mimeType || 'audio/webm' })
        releaseStream()
        if (!mountedRef.current) return
        if (durationMs < MIN_RECORDING_MS || blob.size === 0) {
          setStatus('idle')
          setError(t('composerVoiceTooShort'))
          return
        }
        setStatus('transcribing')
        void transcribeBlob(blob, durationMs, intent)
      }
      try {
        const audioContext = new AudioContext()
        const analyser = audioContext.createAnalyser()
        analyser.fftSize = 512
        analyser.smoothingTimeConstant = 0.55
        audioContext.createMediaStreamSource(stream).connect(analyser)
        audioContextRef.current = audioContext
        analyserRef.current = analyser
        levelDataRef.current = new Uint8Array(new ArrayBuffer(analyser.fftSize))
      } catch {
        // 波形只是视觉反馈,拿不到 analyser 也不影响录音本身。
      }
      streamRef.current = stream
      recorderRef.current = recorder
      stopIntentRef.current = 'insert'
      startedAtRef.current = Date.now()
      setStartedAtMs(startedAtRef.current)
      recorder.start()
      setStatus('recording')
      maxDurationTimerRef.current = window.setTimeout(() => {
        if (recorderRef.current?.state === 'recording') recorderRef.current.stop()
      }, SPEECH_TRANSCRIPTION_MAX_DURATION_MS)
    })()
  }, [releaseStream, t, transcribeBlob])

  const stop = useCallback((intent: VoiceDictationIntent = 'insert'): void => {
    if (recorderRef.current?.state === 'recording') {
      stopIntentRef.current = intent
      recorderRef.current.stop()
    }
  }, [])

  const toggle = useCallback((): void => {
    if (status === 'recording') {
      stop()
    } else if (status === 'idle') {
      start()
    }
  }, [start, status, stop])

  return { status, error, startedAtMs, start, stop, toggle, getLevel }
}

/**
 * MediaRecorder yields webm/opus, but speech providers expect a plain
 * audio file. Decode and resample to mono 16 kHz 16-bit WAV, the common
 * denominator for OpenAI transcriptions and MiMo ASR.
 */
async function encodeBlobAsWav(blob: Blob): Promise<{ base64: string }> {
  const compressed = await blob.arrayBuffer()
  const decodeContext = new AudioContext()
  let decoded: AudioBuffer
  try {
    decoded = await decodeContext.decodeAudioData(compressed)
  } finally {
    void decodeContext.close()
  }
  const frameCount = Math.max(1, Math.ceil(decoded.duration * TRANSCRIPTION_SAMPLE_RATE))
  const offline = new OfflineAudioContext(1, frameCount, TRANSCRIPTION_SAMPLE_RATE)
  const source = offline.createBufferSource()
  source.buffer = decoded
  source.connect(offline.destination)
  source.start()
  const rendered = await offline.startRendering()
  const wavBytes = encodeWavPcm16(rendered.getChannelData(0), TRANSCRIPTION_SAMPLE_RATE)
  return { base64: bytesToBase64(wavBytes) }
}

function encodeWavPcm16(samples: Float32Array, sampleRate: number): Uint8Array {
  const dataLength = samples.length * 2
  const buffer = new ArrayBuffer(44 + dataLength)
  const view = new DataView(buffer)
  const writeAscii = (offset: number, text: string): void => {
    for (let i = 0; i < text.length; i += 1) view.setUint8(offset + i, text.charCodeAt(i))
  }
  writeAscii(0, 'RIFF')
  view.setUint32(4, 36 + dataLength, true)
  writeAscii(8, 'WAVE')
  writeAscii(12, 'fmt ')
  view.setUint32(16, 16, true)
  view.setUint16(20, 1, true)
  view.setUint16(22, 1, true)
  view.setUint32(24, sampleRate, true)
  view.setUint32(28, sampleRate * 2, true)
  view.setUint16(32, 2, true)
  view.setUint16(34, 16, true)
  writeAscii(36, 'data')
  view.setUint32(40, dataLength, true)
  let offset = 44
  for (let i = 0; i < samples.length; i += 1) {
    const clamped = Math.max(-1, Math.min(1, samples[i]))
    view.setInt16(offset, clamped < 0 ? clamped * 0x8000 : clamped * 0x7fff, true)
    offset += 2
  }
  return new Uint8Array(buffer)
}

function bytesToBase64(bytes: Uint8Array): string {
  let binary = ''
  const chunkSize = 0x8000
  for (let i = 0; i < bytes.length; i += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(i, i + chunkSize))
  }
  return btoa(binary)
}
