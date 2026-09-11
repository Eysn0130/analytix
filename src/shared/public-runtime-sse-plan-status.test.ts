import { describe, expect, it } from 'vitest'
import { PublicRuntimeEventFilter } from './public-runtime-content'
import {
  isStrictPublicRuntimeSseIpcPayload,
  projectPublicRuntimeSseBlock
} from './public-runtime-sse'

function frame(event: Record<string, unknown>): string {
  return `id: ${event.seq}\nevent: ${event.kind}\ndata: ${JSON.stringify(event)}`
}

function createPlanFinishedEvent(relativePath = '.analytixsdd/plan/auth.md'): Record<string, unknown> {
  return {
    seq: 47,
    kind: 'tool_call_finished',
    timestamp: '2026-08-01T00:00:00.000Z',
    threadId: 'thread-plan',
    turnId: 'turn-plan',
    itemId: 'tool-result-plan',
    item: {
      id: 'tool-result-plan',
      turnId: 'turn-plan',
      threadId: 'thread-plan',
      role: 'tool',
      status: 'completed',
      createdAt: '2026-08-01T00:00:00.000Z',
      finishedAt: '2026-08-01T00:00:01.000Z',
      kind: 'tool_result',
      toolName: 'create_plan',
      callId: 'host-call-plan',
      toolKind: 'file_change',
      isError: false,
      output: {
        schemaVersion: 1,
        projectionKind: 'plan_status',
        disclosure: 'metadata_only',
        status: 'completed',
        code: 'plan_updated',
        messageKey: 'plan_updated',
        privatePayloadWithheld: true,
        factAnswerAllowed: false,
        evidenceAuthority: false,
        plan: {
          planId: 'plan-auth',
          relativePath,
          operation: 'draft',
          contentHash: 'a'.repeat(64),
          byteSize: 128,
          savedAt: '2026-08-01T00:00:01.000Z'
        }
      }
    }
  }
}

describe('public runtime SSE plan status', () => {
  it('admits the canonical metadata-only create_plan result', () => {
    const event = createPlanFinishedEvent()

    expect(projectPublicRuntimeSseBlock(
      frame(event),
      'thread-plan',
      new PublicRuntimeEventFilter()
    )).toEqual({ status: 'emit', seq: 47, event })
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'stream-plan',
      events: [event]
    })).toBe(true)
  })

  it.each([
    '/tmp/private-plan.md',
    '../../private-plan.md',
    '.analytixsdd/plan/6222021234567890123.md'
  ])('rejects an unsafe or protected plan path: %s', (relativePath) => {
    const event = createPlanFinishedEvent(relativePath)

    expect(projectPublicRuntimeSseBlock(
      frame(event),
      'thread-plan',
      new PublicRuntimeEventFilter()
    )).toEqual({ status: 'invalid', reason: 'invalid_public_projection' })
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'stream-plan',
      events: [event]
    })).toBe(false)
  })
})
