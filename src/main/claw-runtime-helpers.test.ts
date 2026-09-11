import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  feishuSenderLabel,
  finalAssistantReplyText,
  imCompletionReplyForPush,
  IM_COMPLETED_NO_TEXT_REPLY,
  subscribeRuntimeThreadEvents,
  type ThreadDetailJson,
  type TurnItemJson
} from './claw-runtime-helpers'

afterEach(() => {
  vi.unstubAllGlobals()
})

async function waitFor(predicate: () => boolean): Promise<void> {
  for (let index = 0; index < 100; index += 1) {
    if (predicate()) return
    await new Promise<void>((resolve) => setTimeout(resolve, 0))
  }
  expect(predicate()).toBe(true)
}

function singleTurnDetail(items: TurnItemJson[]): ThreadDetailJson {
  return { turns: [{ id: 'turn_1', status: 'completed', items }] }
}

describe('finalAssistantReplyText', () => {
  it('returns the concluding text that follows the last tool activity', () => {
    const detail = singleTurnDetail([
      { kind: 'assistant_text', text: '我的计划：先读文件，再修改' },
      { kind: 'tool_call' },
      { kind: 'tool_result' },
      { kind: 'assistant_text', text: '已完成：结果是 42' }
    ])
    expect(finalAssistantReplyText(detail, { turnId: 'turn_1' })).toBe('已完成：结果是 42')
  })

  it('skips the pre-tool plan when the turn ends without concluding text', () => {
    // The exact bug: the model narrates a plan as text, performs the work
    // through tools, and stops without a final message. The plan must not
    // be mistaken for the result.
    const detail = singleTurnDetail([
      { kind: 'assistant_reasoning', text: '正在思考……' },
      { kind: 'assistant_text', text: '我的计划：先读文件，再修改' },
      { kind: 'tool_call' },
      { kind: 'tool_result' }
    ])
    expect(finalAssistantReplyText(detail, { turnId: 'turn_1' })).toBe('')
  })

  it('never treats reasoning as the reply', () => {
    const detail = singleTurnDetail([
      { kind: 'assistant_reasoning', text: '思考：结论应该是 X' },
      { kind: 'tool_call' },
      { kind: 'tool_result' },
      { kind: 'assistant_reasoning', text: '结束思考：已经完整完成 X' }
    ])
    expect(finalAssistantReplyText(detail, { turnId: 'turn_1' })).toBe('')
  })

  it('returns the last message for a pure chat turn with no tools', () => {
    const detail = singleTurnDetail([
      { kind: 'assistant_text', text: '第一段' },
      { kind: 'assistant_text', text: '最终答案' }
    ])
    expect(finalAssistantReplyText(detail, { turnId: 'turn_1' })).toBe('最终答案')
  })

  it('withholds archived assistant text containing provider think markup', () => {
    const detail = singleTurnDetail([
      { kind: 'assistant_text', text: '公开前缀<think>PRIVATE_REASONING</think>公开结论' }
    ])
    expect(finalAssistantReplyText(detail, { turnId: 'turn_1' })).toBe('')
  })

  it('withholds restricted evidence and closed refs from a completed reply', () => {
    const sourceRowRef = `srow1_${'b'.repeat(64)}`
    const canonicalEvidenceV3 = JSON.stringify({
      canonicalEvidence: {
        schemaVersion: 3,
        purpose: 'analytix.canonical-evidence/v3',
        facts: []
      }
    })

    expect(finalAssistantReplyText(singleTurnDetail([
      { kind: 'assistant_text', text: `prefix ${sourceRowRef} suffix` }
    ]), { turnId: 'turn_1' })).toBe('')
    expect(finalAssistantReplyText(singleTurnDetail([
      { kind: 'assistant_text', text: `prefix ${canonicalEvidenceV3} suffix` }
    ]), { turnId: 'turn_1' })).toBe('')
  })

  it('scopes extraction to the requested turn and ignores earlier turns', () => {
    const detail: ThreadDetailJson = {
      turns: [
        { id: 'turn_prev', status: 'completed', items: [{ kind: 'assistant_text', text: '旧回复' }] },
        { id: 'turn_cur', status: 'completed', items: [{ kind: 'tool_call' }, { kind: 'tool_result' }] }
      ]
    }
    expect(finalAssistantReplyText(detail, { turnId: 'turn_cur' })).toBe('')
    expect(finalAssistantReplyText(detail, { turnId: 'turn_prev' })).toBe('旧回复')
  })
})

describe('imCompletionReplyForPush', () => {
  it('is the plain completion note when no files were produced', () => {
    expect(imCompletionReplyForPush([])).toBe(IM_COMPLETED_NO_TEXT_REPLY)
  })

  it('lists generated file names so they can be retrieved later', () => {
    const reply = imCompletionReplyForPush([
      { path: '/w/a.md', fileName: 'a.md' },
      { path: '/w/b.png', fileName: 'b.png' }
    ])
    expect(reply).toContain('a.md')
    expect(reply).toContain('b.png')
  })
})

describe('feishuSenderLabel', () => {
  it('falls back when sender fields are missing', () => {
    expect(feishuSenderLabel({} as Parameters<typeof feishuSenderLabel>[0])).toBe('feishu-user')
  })

  it('prefers senderName over senderId', () => {
    expect(feishuSenderLabel({
      senderName: ' Alice ',
      senderId: 'ou_123'
    } as Parameters<typeof feishuSenderLabel>[0])).toBe('Alice')
  })
})

describe('subscribeRuntimeThreadEvents', () => {
  it('reuses the strict SSE boundary and fails closed before Connect Phone sees a mismatched event', async () => {
    const valid = {
      seq: 1,
      kind: 'tool_progress',
      timestamp: '2026-07-14T00:00:00.000Z',
      threadId: 'thread-connect-phone',
      turnId: 'turn-connect-phone',
      toolName: 'test_tool',
      callId: 'call-1',
      status: 'running'
    }
    const privateMismatch = {
      seq: 9,
      kind: 'heartbeat',
      timestamp: '2026-07-14T00:00:00.000Z',
      threadId: 'other-thread',
      privatePayload: 'CONNECT_PHONE_PRIVATE_SENTINEL'
    }
    const body = [
      `id: 1\nevent: tool_progress\ndata: ${JSON.stringify(valid)}\n\n`,
      `id: 9\nevent: heartbeat\ndata: ${JSON.stringify(privateMismatch)}\n\n`
    ].join('')
    const fetchMock = vi.fn(async () => new Response(body, {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    }))
    vi.stubGlobal('fetch', fetchMock)
    const onEvent = vi.fn()
    const logError = vi.fn()
    const controller = new AbortController()

    const handle = await subscribeRuntimeThreadEvents({
      baseUrl: 'http://127.0.0.1:9988',
      threadId: 'thread-connect-phone',
      headers: { Authorization: 'Bearer redacted-test-token' },
      onEvent,
      signal: controller.signal,
      logError
    })
    await waitFor(() => logError.mock.calls.some(([, message]) => String(message).includes('event rejected')))

    expect(onEvent).toHaveBeenCalledTimes(2)
    expect(onEvent).toHaveBeenNthCalledWith(1, valid)
    expect(onEvent).toHaveBeenNthCalledWith(2, {
      schemaVersion: 1,
      kind: 'public_projection_revoked',
      threadId: 'thread-connect-phone',
      historyAuthority: 'case_boundary_only_v1',
      code: 'case_public_authority_unavailable',
      action: 'purge_case_projection',
      terminal: true
    })
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(JSON.stringify(onEvent.mock.calls)).not.toContain('CONNECT_PHONE_PRIVATE_SENTINEL')
    expect(logError).toHaveBeenCalledWith(
      'sse',
      'SSE event rejected',
      { code: 'sse_event_rejected', reasonCode: 'event_thread_mismatch' }
    )

    handle.close()
  })
})
