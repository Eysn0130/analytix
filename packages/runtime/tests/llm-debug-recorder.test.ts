import { describe, expect, it } from 'vitest'
import { LlmDebugRecorder } from '../src/services-test-support/llm-debug-recorder.js'

function record(recorder: LlmDebugRecorder, model: string): void {
  const round = recorder.start({ threadId: 't', turnId: 'u', provider: 'compat', model })
  round.requestBody = { model }
  round.output.text = `out:${model}`
  recorder.finish(round)
}

describe('LlmDebugRecorder', () => {
  it('keeps only the most recent 25 rounds', () => {
    const recorder = new LlmDebugRecorder()
    for (let i = 1; i <= 30; i++) record(recorder, `m${i}`)
    const snapshot = recorder.snapshot()
    expect(snapshot).toHaveLength(25)
    // Oldest five (m1..m5) dropped; m6 is the oldest retained.
    expect(snapshot[snapshot.length - 1]?.model).toBe('m6')
  })

  it('returns the snapshot most-recent first', () => {
    const recorder = new LlmDebugRecorder()
    record(recorder, 'a')
    record(recorder, 'b')
    const snapshot = recorder.snapshot()
    expect(snapshot.map((r) => r.model)).toEqual(['b', 'a'])
    expect(snapshot[0]?.requestBody).toEqual({ model: 'b' })
    expect(snapshot[0]?.output.text).toBe('out:b')
  })

  it('clear empties the buffer', () => {
    const recorder = new LlmDebugRecorder()
    record(recorder, 'a')
    recorder.clear()
    expect(recorder.snapshot()).toHaveLength(0)
  })
})
