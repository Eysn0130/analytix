import { describe, expect, it } from 'vitest'
import { ANALYTIX_RUNTIME_TOOL_EXECUTIONS_OBSERVE_PATH } from '../../shared/analytix-endpoints'
import { sanitizeRuntimeResponse } from './analytix-adapter'

const digest = (value: string): string => value.repeat(64)

const observation = {
  schemaVersion: 1,
  disclosure: 'metadata_only',
  privatePayloadWithheld: true,
  threadId: 'thread-1',
  turnId: 'turn-1',
  toolName: 'bash',
  status: 'completed',
  workId: digest('1'),
  receiptId: digest('2'),
  dispositionId: digest('3'),
  executionGrantId: digest('4'),
  resultItemId: `item_result_${digest('5')}`,
  resultItemDigest: digest('6')
} as const

describe('tool execution observation public response', () => {
  it('accepts the exact metadata-only host observation', () => {
    const accepted = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify(observation)
    }, ANALYTIX_RUNTIME_TOOL_EXECUTIONS_OBSERVE_PATH, null, 'POST')

    expect(accepted.ok, accepted.body).toBe(true)
    expect(JSON.parse(accepted.body)).toEqual(observation)
  })

  it('rejects payload data and contract drift without reflecting it', () => {
    const privateSentinel = 'PRIVATE command and output must not cross main'
    for (const candidate of [
      { ...observation, output: privateSentinel },
      { ...observation, privatePayloadWithheld: false, command: privateSentinel },
      { ...observation, resultItemDigest: 'not-a-digest', workspace: privateSentinel },
      { ...observation, toolName: 'funds', arguments: privateSentinel }
    ]) {
      const rejected = sanitizeRuntimeResponse({
        ok: true,
        status: 200,
        body: JSON.stringify(candidate)
      }, ANALYTIX_RUNTIME_TOOL_EXECUTIONS_OBSERVE_PATH, null, 'POST')

      expect(rejected.ok).toBe(false)
      expect(rejected.status).toBe(502)
      expect(rejected.body).not.toContain(privateSentinel)
    }

    const empty = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: ''
    }, ANALYTIX_RUNTIME_TOOL_EXECUTIONS_OBSERVE_PATH, null, 'POST')
    expect(empty.ok).toBe(false)
    expect(empty.status).toBe(502)
  })
})
