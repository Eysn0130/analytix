import { describe, expect, it } from 'vitest'
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
