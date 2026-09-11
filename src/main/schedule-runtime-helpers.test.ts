import { describe, expect, it } from 'vitest'
import {
  latestAssistantText,
  runtimeErrorMessage,
  type ThreadDetailJson
} from './schedule-runtime-helpers'

describe('schedule runtime public text boundary', () => {
  it('withholds task history and summaries containing provider think markup', () => {
    const detail: ThreadDetailJson = {
      turns: [{
        id: 'turn-1',
        status: 'completed',
        items: [{
          kind: 'assistant_text',
          text: '公开<think>PRIVATE_REASONING</think>结论'
        }]
      }]
    }
    expect(latestAssistantText(detail, { turnId: 'turn-1' })).toBe('')
  })

  it('always replaces runtime response bodies with the host fallback', () => {
    expect(runtimeErrorMessage({
      ok: false,
      status: 500,
      body: JSON.stringify({ message: '<think>PRIVATE_REASONING</think>' })
    }, 'Runtime failed.')).toBe('Runtime failed.')
    expect(runtimeErrorMessage({
      ok: false,
      status: 502,
      body: JSON.stringify({ error: { message: 'MARKER_FREE_PROVIDER_ERROR_SENTINEL' } })
    }, 'Runtime failed.')).toBe('Runtime failed.')
    expect(runtimeErrorMessage({
      ok: false,
      status: 503,
      body: 'MARKER_FREE_RUNTIME_BODY_SENTINEL'
    }, 'Runtime failed.')).toBe('Runtime failed.')
  })
})
