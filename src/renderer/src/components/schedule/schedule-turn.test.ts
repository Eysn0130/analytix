import { describe, expect, it } from 'vitest'
import type { ChatBlock, ToolBlock } from '../../agent/types'
import { nativeScheduleOperation, scheduleCreationCalls } from './schedule-turn'

function call(callId: string): ToolBlock {
  const threadId = 'thread', turnId = 'turn', callItem = 'call-' + callId, resultItem = 'result-' + callId
  return { kind: 'tool', id: callId, createdAt: '2099-01-01T00:00:00.000Z', summary: 'Schedule', status: 'success', toolKind: 'tool_call', meta: {
    toolName: 'mcp__gui_schedule__gui_schedule_create', recordThreadId: threadId, turnId, callId,
    toolCallItemId: callItem, toolResultItemId: resultItem,
    toolCallIdentity: { threadId, turnId, callId, itemId: callItem },
    toolResultIdentity: { threadId, turnId, callId, itemId: resultItem },
    localResult: { turnId, callId, resultItemId: resultItem }
  } }
}
describe('ended Schedule receipt candidates', () => {
  it('accepts only the six real native producers and includes both GUI and Claw creation', () => {
    for (const namespace of ['gui', 'claw']) for (const operation of ['create', 'update', 'delete']) {
      expect(nativeScheduleOperation(`mcp__gui_schedule__${namespace}_schedule_${operation}`)).toBe(operation)
    }
    expect(nativeScheduleOperation('schedule_create')).toBeNull()
    const claw = call('claw'); claw.meta!.toolName = 'mcp__gui_schedule__claw_schedule_create'
    expect(scheduleCreationCalls('thread', 'turn', [call('gui'), claw], true)).toHaveLength(2)
  })
  it('retains earlier successful pairs without requiring prose after the end boundary', () => {
    const successful = call('a'), failed = { ...call('b'), status: 'error' as const }
    expect(scheduleCreationCalls('thread', 'turn', [successful, failed], true).map(row => row.block.id)).toEqual(['a'])
    expect(scheduleCreationCalls('thread', 'turn', [successful], false)).toEqual([])
  })
  it('deduplicates exact identities and preserves distinct calls regardless of task ID', () => {
    const a = call('a'), b = call('b')
    expect(scheduleCreationCalls('thread', 'turn', [a, { ...a }, b], true).map(row => row.block.id)).toEqual(['a', 'b'])
  })
  it('rejects missing result/call witnesses, foreign turns, mismatched locators and running calls', () => {
    const a = call('a')
    const blocks: ChatBlock[] = [
      { ...a, status: 'running' },
      { ...a, meta: { ...a.meta, child: { parentThreadId: 'thread', parentTurnId: 'turn', childId: 'child', childStatus: 'completed' } } },
      { ...a, meta: { ...a.meta, toolCallItemId: undefined } },
      { ...a, meta: { ...a.meta, toolResultItemId: undefined } },
      { ...a, meta: { ...a.meta, localResult: { turnId: 'other', callId: 'a', resultItemId: 'result-a' } } },
      { ...a, meta: { ...a.meta, toolName: 'schedule_update' } }
    ]
    expect(scheduleCreationCalls('thread', 'turn', blocks, true)).toEqual([])
    expect(scheduleCreationCalls('thread', 'other', [a], true)).toEqual([])
    expect(scheduleCreationCalls('other', 'turn', [a], true)).toEqual([])
  })
})
