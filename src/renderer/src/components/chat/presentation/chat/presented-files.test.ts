import { describe, expect, it } from 'vitest'
import type { ChatBlock, ToolBlock } from '../../../../agent/types'
import type { ToolResultLocalPresentHistoryV1 } from '../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import { latestPresentedFiles, presentCallsForClosing, presentedBasename, presentedDescription } from './presented-files'
const anchor = (key: string): ToolResultLocalPresentHistoryV1['orderedItems'][number] => ({
  kind: 'tool_result', toolName: 'present', callId: 'call_host_' + key.repeat(64), itemId: 'item_result_' + key.repeat(64)
})
const closing = (id: string): ChatBlock => ({ kind: 'assistant', id, text: 'Actual answer', meta: { turnId: 'turn' } })
const history: ToolResultLocalPresentHistoryV1 = { schemaVersion: 1, kind: 'present_history', threadId: 'thread', turnId: 'turn',
  orderedItems: [anchor('b'), { kind: 'assistant_text', itemId: 'early' }, anchor('a'), { kind: 'assistant_text', itemId: 'late' }, anchor('c')] }
describe('private primary-order Present tail selection', () => {
  it('uses true result order for each actual closing rather than tool call seat order', () => {
    const first = presentCallsForClosing('thread', 'turn', [], history, closing('early'), false)
    const last = presentCallsForClosing('thread', 'turn', [], history, closing('late'), true)
    expect(first.map(call => call.block.meta?.callId)).toEqual(['call_host_' + 'b'.repeat(64)])
    expect(last.map(call => call.block.meta?.callId)).toEqual(['call_host_' + 'b'.repeat(64), 'call_host_' + 'a'.repeat(64)])
    expect(last[0].block.meta).not.toHaveProperty('seq')
    expect(last[0].block.meta).not.toHaveProperty('callItemId')
  })
  it('consumes the same private anchors when case history strips all tool blocks', () => {
    const caseBlocks = [closing('late')]
    const calls = presentCallsForClosing('thread', 'turn', caseBlocks, history, caseBlocks[0], true)
    expect(calls).toHaveLength(2)
    expect(calls[0].block.meta?.localResult).toEqual({ turnId: 'turn', callId: 'call_host_' + 'b'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64) })
    const original: ToolBlock = { ...calls[0].block, id: 'original-call-seat' }
    expect(presentCallsForClosing('thread', 'turn', [original], history, closing('early'), true)[0].block).toBe(original)
    expect(caseBlocks).toHaveLength(1)
  })
  it('does not treat draft/partial/foreign text as closing, and requires real end when no approved closing exists', () => {
    for (const block of [closing('draft'), { ...closing('early'), text: ' ' }, { ...closing('early'), meta: { turnId: 'other' } }]) {
      expect(presentCallsForClosing('thread', 'turn', [], history, block, true)).toEqual([])
    }
    expect(presentCallsForClosing('other', 'turn', [], history, closing('early'), true)).toEqual([])
    expect(presentCallsForClosing('thread', 'turn', [], null, closing('early'), true)).toEqual([])
    expect(presentCallsForClosing('thread', 'turn', [], history, null, true)).toEqual([])
    const stopped = { ...history, orderedItems: [anchor('a'), anchor('b')] }
    expect(presentCallsForClosing('thread', 'turn', [], stopped, null, false)).toEqual([])
    expect(presentCallsForClosing('thread', 'turn', [], stopped, null, true)).toHaveLength(2)
  })
  it('keeps the first native-object position and last declaration, without basename/hash equality', () => {
    const files = [{ key: 'a', fileIdentity: 'same', description: 'old' }, { key: 'b', fileIdentity: 'different', description: 'other' },
      { key: 'c', fileIdentity: 'same', description: 'latest' }, { key: 'd' }]
    expect(latestPresentedFiles(files)).toEqual([files[2], files[1], files[3]])
    expect(presentedBasename('a\\b/actual.md')).toBe('actual.md')
    expect(presentedDescription('Actual report (Markdown)', 'actual.md')).toBe('Actual report')
    expect(presentedDescription('', 'actual.md')).toBe('MD')
    expect(presentedDescription(undefined, 'README')).toBe('FILE')
  })
})
