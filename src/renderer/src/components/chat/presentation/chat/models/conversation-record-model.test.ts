import { describe, expect, it } from 'vitest'
import { chatBlockFromItem, mergeChatBlocks } from '../../../../../agent/analytix-mapper'
import type { CoreTurnItemJson } from '../../../../../agent/analytix-contract'
import type { ChatBlock } from '../../../../../agent/types'
import { conversationCallRecords, conversationRecordTurns, locateConversationCall } from './conversation-record-model'

const base = { threadId: 'thread', turnId: 'turn', callId: 'call', toolName: 'get_goal', toolKind: 'tool_call', createdAt: '2026-10-09T00:00:00Z' } as const
const args = { schemaVersion: 1, projectionKind: 'withheld', disclosure: 'metadata_only', messageKey: 'tool_arguments_withheld',
  privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false } as const
const output = { schemaVersion: 1, projectionKind: 'host_status', disclosure: 'metadata_only', status: 'completed', code: 'tool_completed',
  messageKey: 'tool_completed', privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false } as const
const call: CoreTurnItemJson = { ...base, id: 'physical-call', kind: 'tool_call', role: 'assistant', status: 'running', arguments: args }
const result: CoreTurnItemJson = { ...base, id: 'physical-result', kind: 'tool_result', role: 'tool', status: 'completed', isError: false, output }
const map = (item: CoreTurnItemJson): ChatBlock => { const block = chatBlockFromItem(item); if (!block) throw new Error('fixture must map'); return block }

describe('admitted conversation record identity', () => {
  it('folds an actual call/result pair and retains both physical identities in source order', () => {
    const blocks = mergeChatBlocks([map(call), map(result)])
    expect(blocks).toHaveLength(1)
    const records = conversationCallRecords('thread', blocks)
    expect(records).toHaveLength(1)
    expect(records[0]).toMatchObject({ threadId: 'thread', turnId: 'turn', callId: 'call', toolCallItemId: call.id, toolResultItemId: result.id })
    expect(locateConversationCall(records, base)).toEqual({ kind: 'found', record: records[0] })
    expect(JSON.stringify(records)).not.toContain('argsRaw')
  })
  it('does not infer physical item kind from ready/progress/legacy payloads', () => {
    for (const item of [{ ...call, arguments: 'RAW_ARGS' }, { ...result, output: { raw: 'RAW_RESULT' } },
      { ...call, kind: 'tool_progress' as const, arguments: undefined }]) {
      expect(conversationCallRecords('thread', [map(item)])).toEqual([])
    }
  })
  it('rejects cross-thread/turn pairs even when a caller reuses callId and the existing merge key', () => {
    for (const changed of [{ turnId: 'other-turn' }, { threadId: 'other-thread' }]) {
      const blocks = mergeChatBlocks([map(call), map({ ...result, ...changed })])
      expect(conversationCallRecords(changed.threadId ?? 'thread', blocks)).toEqual([])
    }
  })
  it('keeps missing and ambiguous requests pending instead of selecting another result', () => {
    const record = conversationCallRecords('thread', mergeChatBlocks([map(call), map(result)]))[0]
    expect(locateConversationCall([record], { ...base, turnId: 'absent' })).toEqual({ kind: 'missing' })
    expect(locateConversationCall([record, record], base)).toEqual({ kind: 'ambiguous' })
    expect(locateConversationCall([record], { ...base, toolResultItemId: 'other-result' })).toEqual({ kind: 'missing' })
  })
  it('uses physical IDs to distinguish exact rows without conflating equal logical call labels', () => {
    const a = conversationCallRecords('thread', [map(result)])[0]
    const b = conversationCallRecords('thread', [map({ ...result, id: 'second-result' })])[0]
    expect(a.key).not.toBe(b.key)
    expect(locateConversationCall([a, b], base)).toEqual({ kind: 'ambiguous' })
    expect(locateConversationCall([a, b], { ...base, toolResultItemId: b.toolResultItemId })).toEqual({ kind: 'found', record: b })
  })
  it.each([[0x1f, false], [0x20, true], [0x7f, false], [0x80, true]] as const)('preserves the control-code boundary for local identity %#', (code, admitted) => {
    const threadId = 'thread' + String.fromCharCode(code)
    const source = map({ ...call, threadId })
    expect(conversationCallRecords(threadId, [source])).toHaveLength(admitted ? 1 : 0)
  })
  it('groups late records under their real native turn rather than the neighboring user or Assistant', () => {
    const first: ChatBlock = { kind: 'user', id: 'user-old', text: 'Old admitted request', meta: { turnId: 'turn' } }
    const next: ChatBlock = { kind: 'user', id: 'user-next', text: 'Next admitted request', meta: { turnId: 'next-turn' } }
    const late = map(result)
    const turns = conversationRecordTurns('thread', [first, next, late])
    expect(turns).toHaveLength(2)
    expect(turns[0].user).toBe(first); expect(turns[0].blocks).toEqual([late])
    expect(turns[1].user).toBe(next); expect(turns[1].blocks).toEqual([])
  })
})
