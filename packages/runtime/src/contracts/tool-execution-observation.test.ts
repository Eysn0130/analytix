import { describe, expect, it } from 'vitest'
import { ToolCallReadyEvent } from './events.js'
import { SuccessfulToolExecutionObservationV1 } from './tool-execution-observation.js'

const digest = (value: string): string => value.repeat(64)

function fixture(): Record<string, unknown> {
  return {
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
  }
}

describe('SuccessfulToolExecutionObservationV1', () => {
  it('accepts only the closed metadata-only projection', () => {
    expect(SuccessfulToolExecutionObservationV1.safeParse(fixture()).success).toBe(true)
    expect(SuccessfulToolExecutionObservationV1.safeParse({
      ...fixture(),
      output: 'private output'
    }).success).toBe(false)
    expect(SuccessfulToolExecutionObservationV1.safeParse({
      ...fixture(),
      resultItemId: 'item_result_not-a-digest'
    }).success).toBe(false)
  })
})


describe('tool readiness event presence', () => {
  it('keeps preparation partial and the durable item identity distinct on replay', () => {
    const partial = ToolCallReadyEvent.parse({
      kind: 'tool_call_ready', seq: 1, timestamp: '2026-10-09T00:00:00Z',
      threadId: 'thread-1', turnId: 'turn-1', callId: 'call-1', toolName: 'bash', readyCount: 1, partial: true
    })
    expect(partial.partial).toBe(true)
    expect(partial.itemId).toBeUndefined()
    const ready = ToolCallReadyEvent.parse({ ...partial, seq: 2, partial: false, itemId: 'item-call' })
    expect(ready.partial).toBe(false)
    expect(ready.itemId).toBe('item-call')
  })
})
