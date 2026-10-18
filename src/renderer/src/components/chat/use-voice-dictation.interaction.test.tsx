// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type { SpeechTranscriptionResult } from '@shared/speech-to-text'
import { useVoiceDictation } from './use-voice-dictation'

let root: Root, host: HTMLDivElement, voice: ReturnType<typeof useVoiceDictation>
let bridge: PropertyDescriptor | undefined
const delivered = vi.fn()
const trackStop = vi.fn()
let permission: ReturnType<typeof vi.fn<() => Promise<MediaStream>>>
let transcribe: ReturnType<typeof vi.fn<() => Promise<SpeechTranscriptionResult>>>
const recorders: FakeRecorder[] = []
const stream = () => ({ getTracks: () => [{ stop: trackStop }] }) as unknown as MediaStream
class AudioBlob {
  size = 8
  async arrayBuffer() { return new ArrayBuffer(8) }
}
class FakeRecorder {
  static isTypeSupported() { return true }
  state: RecordingState = 'inactive'
  mimeType = 'audio/webm'
  ondataavailable: ((event: BlobEvent) => void) | null = null
  onstop: ((event: Event) => void) | null = null
  constructor() { recorders.push(this) }
  start() { this.state = 'recording' }
  stop() {
    this.state = 'inactive'
    this.ondataavailable?.({ data: new AudioBlob() } as unknown as BlobEvent)
    this.onstop?.(new Event('stop'))
  }
}
class Audio {
  async close() {}
  async decodeAudioData() { return { duration: 0.01 } }
  createAnalyser() { return { fftSize: 512, smoothingTimeConstant: 0, getByteTimeDomainData() {} } }
  createMediaStreamSource() { return { connect() {} } }
}
class OfflineAudio {
  destination = {}
  createBufferSource() { return { buffer: null, connect() {}, start() {} } }
  async startRendering() { return { getChannelData: () => new Float32Array(160) } }
}
function Probe({ owner }: { owner: string }) {
  voice = useVoiceDictation({ ownerKey: owner, onText: delivered })
  return null
}
async function render(owner: string) { await act(async () => root.render(createElement(Probe, { owner }))) }
async function settle() { await act(async () => { for (let n = 0; n < 20; n++) await Promise.resolve() }) }
async function finish(intent: 'insert' | 'send' = 'insert') {
  vi.setSystemTime(Date.now() + 750)
  await act(async () => voice.stop(intent)); await settle()
}
beforeEach(() => {
  vi.useFakeTimers(); vi.setSystemTime(10_000); vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  vi.stubGlobal('Blob', AudioBlob); vi.stubGlobal('MediaRecorder', FakeRecorder)
  vi.stubGlobal('AudioContext', Audio); vi.stubGlobal('OfflineAudioContext', OfflineAudio)
  permission = vi.fn(async () => stream()); transcribe = vi.fn<() => Promise<SpeechTranscriptionResult>>(async () => ({ ok: true, text: 'OWNED_TRANSCRIPT' }))
  vi.stubGlobal('navigator', { mediaDevices: { getUserMedia: permission } })
  bridge = Object.getOwnPropertyDescriptor(window, 'analytix')
  Object.defineProperty(window, 'analytix', { configurable: true, value: { speech: { transcribe } } })
  recorders.length = 0; delivered.mockReset(); trackStop.mockReset()
  host = document.createElement('div'); document.body.append(host); root = createRoot(host)
})
afterEach(async () => {
  await act(async () => root.unmount()); host.remove()
  if (bridge) Object.defineProperty(window, 'analytix', bridge); else Reflect.deleteProperty(window, 'analytix')
  vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals()
})
it('delivers current dictation and an owner ticket that retires after A → B → A', async () => {
  await render('agent-a/thread-a/visible'); await act(async () => voice.start()); await finish('send')
  expect(transcribe).toHaveBeenCalledOnce()
  expect(delivered).toHaveBeenCalledWith('OWNED_TRANSCRIPT', 'send', expect.any(Function))
  const ticket = delivered.mock.calls[0][2] as () => boolean
  expect(ticket()).toBe(true)
  await render('agent-b/thread-a/visible'); await render('agent-a/thread-a/visible')
  expect(ticket()).toBe(false)
})
it('drops late transcription after selection retirement, including returning to the same owner', async () => {
  let resolve!: (result: SpeechTranscriptionResult) => void
  transcribe.mockImplementationOnce(() => new Promise(done => { resolve = done }))
  await render('a'); await act(async () => voice.start()); await finish()
  expect(voice.status).toBe('transcribing')
  await render('b'); await render('a')
  await act(async () => resolve({ ok: true, text: 'STALE_TRANSCRIPT' }))
  expect(delivered).not.toHaveBeenCalled(); expect(voice.status).toBe('idle')
})
it('retires a pending microphone permission without recording or affecting a new owner', async () => {
  let resolve!: (value: MediaStream) => void
  permission.mockImplementationOnce(() => new Promise(done => { resolve = done }))
  await render('a'); await act(async () => voice.start()); await render('b')
  await act(async () => resolve(stream()))
  expect(recorders).toHaveLength(0); expect(trackStop).toHaveBeenCalledOnce()
  await act(async () => voice.start())
  expect(recorders).toHaveLength(1); expect(voice.status).toBe('recording')
})
it('detaches an old recorder callback before releasing its resources', async () => {
  await render('a'); await act(async () => voice.start())
  const oldStop = recorders[0].onstop
  await render('b'); await act(async () => voice.start())
  expect(recorders[0].onstop).toBeNull()
  const released = trackStop.mock.calls.length
  await act(async () => oldStop?.(new Event('stop')))
  expect(trackStop).toHaveBeenCalledTimes(released)
  expect(voice.status).toBe('recording'); expect(transcribe).not.toHaveBeenCalled()
})
