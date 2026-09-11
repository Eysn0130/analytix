import { describe, expect, it } from 'vitest'
import { applyRequestHistoryHygiene } from '../src/shared/request-history-hygiene.js'
import {
  makePrivateToolCallItem as makeToolCallItem,
  makePrivateToolResultItem as makeToolResultItem
} from '../src/domain/item.js'

describe('request history hygiene', () => {
  it('shrinks oversized tool results while preserving head, signal lines, and tail', () => {
    const longOutput = Array.from({ length: 500 }, (_, index) => {
      if (index === 240) return 'ERROR failed to compile auth middleware'
      return `plain output line ${index}`
    }).join('\n')
    const result = makeToolResultItem({
      id: 'result',
      threadId: 'thr_1',
      turnId: 'turn_1',
      callId: 'call_bash',
      toolName: 'bash',
      output: { output: longOutput }
    })

    const compacted = applyRequestHistoryHygiene([result], {
      maxToolResultLines: 80,
      maxToolResultBytes: 4 * 1024
    })
    const compactedResult = compacted[0]
    const originalText = result.kind === 'tool_result' ? JSON.stringify(result.output) : ''
    const compactedText = compactedResult?.kind === 'tool_result'
      ? JSON.stringify(compactedResult.output)
      : ''

    expect(compactedResult).not.toBe(result)
    expect(originalText).toContain('plain output line 499')
    expect(compactedText.length).toBeLessThan(originalText.length)
    expect(compactedText).toContain('plain output line 0')
    expect(compactedText).toContain('ERROR failed to compile auth middleware')
    expect(compactedText).toContain('plain output line 499')
    expect(compactedText).toContain('cache hygiene')
  })

  it('uses tool-shaped snip geometry for read-like and command outputs', () => {
    const readOutput = Array.from({ length: 100 }, (_, index) => `read line ${index}`).join('\n')
    const commandOutput = Array.from({ length: 100 }, (_, index) => `cmd line ${index}`).join('\n')
    const readResult = makeToolResultItem({
      id: 'read_result',
      threadId: 'thr_1',
      turnId: 'turn_1',
      callId: 'call_read',
      toolName: 'read',
      toolKind: 'tool_call',
      output: readOutput
    })
    const commandResult = makeToolResultItem({
      id: 'command_result',
      threadId: 'thr_1',
      turnId: 'turn_1',
      callId: 'call_bash',
      toolName: 'bash',
      toolKind: 'command_execution',
      output: commandOutput
    })

    const compacted = applyRequestHistoryHygiene([readResult, commandResult], {
      maxToolResultLines: 20,
      maxToolResultBytes: 1_000_000,
      maxToolResultTokens: 100_000
    })
    const readText = compacted[0]?.kind === 'tool_result' ? String(compacted[0].output) : ''
    const commandText = compacted[1]?.kind === 'tool_result' ? String(compacted[1].output) : ''

    expect(readText).toContain('read line 12')
    expect(readText).not.toContain('read line 92')
    expect(commandText).not.toContain('cmd line 12')
    expect(commandText).toContain('cmd line 92')
  })

  it('prefers tool-provided snip hints over generic tool-kind geometry', () => {
    const output = Array.from({ length: 100 }, (_, index) => `hinted line ${index}`).join('\n')
    const result = makeToolResultItem({
      id: 'hinted_result',
      threadId: 'thr_1',
      turnId: 'turn_1',
      callId: 'call_hinted',
      toolName: 'custom_command',
      toolKind: 'command_execution',
      output
    })

    const compacted = applyRequestHistoryHygiene([result], {
      maxToolResultLines: 20,
      maxToolResultBytes: 1_000_000,
      maxToolResultTokens: 100_000
    }, {
      toolSnipHints: {
        custom_command: { head: 80, tail: 8, headChars: 10_000, tailChars: 1_000 }
      }
    })
    const text = compacted[0]?.kind === 'tool_result' ? String(compacted[0].output) : ''

    expect(text).toContain('hinted line 12')
    expect(text).not.toContain('hinted line 92')
  })

  it('omits long completed tool-call argument strings only when the result is paired', () => {
    const pairedCall = makeToolCallItem({
      id: 'call_item',
      threadId: 'thr_1',
      turnId: 'turn_1',
      callId: 'call_write',
      toolName: 'write',
      arguments: {
        path: 'src/generated.ts',
        content: 'x'.repeat(12_000)
      }
    })
    const result = makeToolResultItem({
      id: 'result_item',
      threadId: 'thr_1',
      turnId: 'turn_1',
      callId: 'call_write',
      toolName: 'write',
      output: 'wrote src/generated.ts'
    })
    const unpairedCall = makeToolCallItem({
      id: 'unpaired_call_item',
      threadId: 'thr_1',
      turnId: 'turn_1',
      callId: 'call_pending',
      toolName: 'write',
      arguments: { content: 'y'.repeat(12_000) }
    })

    const compacted = applyRequestHistoryHygiene([pairedCall, result, unpairedCall])
    const nextPairedCall = compacted[0]
    const nextUnpairedCall = compacted[2]

    expect(nextPairedCall?.kind === 'tool_call' ? String(nextPairedCall.arguments.content) : '')
      .toContain('cache hygiene')
    expect(nextPairedCall?.kind === 'tool_call' ? nextPairedCall.arguments.path : '')
      .toBe('src/generated.ts')
    expect(nextUnpairedCall?.kind === 'tool_call' ? String(nextUnpairedCall.arguments.content).length : 0)
      .toBe(12_000)
  })

  it('shrinks dense text when the approximate token cap is exceeded before the byte cap', () => {
    const denseOutput = '汉'.repeat(9_000)
    const result = makeToolResultItem({
      id: 'dense_result',
      threadId: 'thr_1',
      turnId: 'turn_1',
      callId: 'call_read',
      toolName: 'read',
      output: { content: denseOutput }
    })

    const compacted = applyRequestHistoryHygiene([result], {
      maxToolResultBytes: 32 * 1024,
      maxToolResultTokens: 4_000
    })
    const compactedResult = compacted[0]
    const compactedText = compactedResult?.kind === 'tool_result'
      ? String((compactedResult.output as { content?: string }).content ?? '')
      : ''

    expect(Buffer.byteLength(denseOutput, 'utf8')).toBeLessThan(32 * 1024)
    expect(compactedText.length).toBeLessThan(denseOutput.length)
    expect(compactedText).toContain('approx')
    expect(compactedText).toContain('cache hygiene')
  })

  it('shrinks completed tool-call args when only the approximate token cap is exceeded', () => {
    const pairedCall = makeToolCallItem({
      id: 'dense_call',
      threadId: 'thr_1',
      turnId: 'turn_1',
      callId: 'call_write',
      toolName: 'write',
      arguments: {
        path: 'src/generated.txt',
        content: '汉'.repeat(2_500)
      }
    })
    const result = makeToolResultItem({
      id: 'dense_result',
      threadId: 'thr_1',
      turnId: 'turn_1',
      callId: 'call_write',
      toolName: 'write',
      output: 'wrote src/generated.txt'
    })

    const compacted = applyRequestHistoryHygiene([pairedCall, result], {
      maxToolArgumentStringBytes: 8 * 1024,
      maxToolArgumentStringTokens: 2_000
    })
    const nextCall = compacted[0]

    expect(nextCall?.kind === 'tool_call' ? String(nextCall.arguments.content) : '')
      .toContain('approx')
    expect(nextCall?.kind === 'tool_call' ? String(nextCall.arguments.content) : '')
      .toContain('cache hygiene')
  })

  it('replaces base64 payloads in model-bound history', () => {
    const result = makeToolResultItem({
      id: 'image_result',
      threadId: 'thr_1',
      turnId: 'turn_1',
      callId: 'call_read',
      toolName: 'read',
      output: { data_base64: 'a'.repeat(2_000), mime: 'image/png' }
    })

    const compacted = applyRequestHistoryHygiene([result])
    const compactedResult = compacted[0]

    expect(compactedResult?.kind === 'tool_result' ? compactedResult.output : {}).toMatchObject({
      data_base64: expect.stringContaining('omitted base64 data'),
      mime: 'image/png'
    })
  })

  it('leaves previous-turn history untouched when scoped to the current turn', () => {
    const previousResult = makeToolResultItem({
      id: 'previous_image_result',
      threadId: 'thr_1',
      turnId: 'turn_previous',
      callId: 'call_previous_read',
      toolName: 'read',
      output: { data_base64: 'a'.repeat(2_000), mime: 'image/png' }
    })
    const currentResult = makeToolResultItem({
      id: 'current_image_result',
      threadId: 'thr_1',
      turnId: 'turn_current',
      callId: 'call_current_read',
      toolName: 'read',
      output: { data_base64: 'b'.repeat(2_000), mime: 'image/png' }
    })

    const compacted = applyRequestHistoryHygiene([previousResult, currentResult], {}, {
      currentTurnId: 'turn_current'
    })
    const compactedPrevious = compacted[0]
    const compactedCurrent = compacted[1]

    expect(compactedPrevious).toBe(previousResult)
    expect(compactedPrevious?.kind === 'tool_result' ? compactedPrevious.output : {}).toMatchObject({
      data_base64: 'a'.repeat(2_000),
      mime: 'image/png'
    })
    expect(compactedCurrent?.kind === 'tool_result' ? compactedCurrent.output : {}).toMatchObject({
      data_base64: expect.stringContaining('omitted base64 data'),
      mime: 'image/png'
    })
  })

  it('uses side-effect-aware guidance when eliding stale tool results', () => {
    const pairedCall = makeToolCallItem({
      id: 'write_call_item',
      threadId: 'thr_1',
      turnId: 'turn_old',
      callId: 'call_write',
      toolName: 'write',
      toolKind: 'file_change',
      arguments: { path: 'src/generated.ts', content: 'small' }
    })
    const writeResult = makeToolResultItem({
      id: 'write_result_item',
      threadId: 'thr_1',
      turnId: 'turn_old',
      callId: 'call_write',
      toolName: 'write',
      toolKind: 'file_change',
      output: 'wrote src/generated.ts\n'.repeat(400)
    })
    const bashResult = makeToolResultItem({
      id: 'bash_result_item',
      threadId: 'thr_1',
      turnId: 'turn_old',
      callId: 'call_bash',
      toolName: 'bash',
      toolKind: 'command_execution',
      output: 'npm test output\n'.repeat(400)
    })

    const compacted = applyRequestHistoryHygiene([pairedCall, writeResult, bashResult], {
      maxCumulativeToolResultTokens: 1,
      keepRecentToolResults: 0,
      maxToolResultTokens: 100_000,
      maxToolResultBytes: 10_000_000,
      maxToolResultLines: 100_000
    })
    const nextCall = compacted[0]
    const nextWriteResult = compacted[1]
    const nextBashResult = compacted[2]
    const writeDigest = nextWriteResult?.kind === 'tool_result' ? String(nextWriteResult.output) : ''
    const bashDigest = nextBashResult?.kind === 'tool_result' ? String(nextBashResult.output) : ''

    expect(nextCall).toBe(pairedCall)
    expect(nextWriteResult).toMatchObject({
      kind: 'tool_result',
      callId: 'call_write',
      toolName: 'write',
      toolKind: 'file_change'
    })
    expect(writeDigest).toContain('older write result elided')
    expect(writeDigest).toContain('avoid re-running mutating tools')
    expect(writeDigest).not.toContain('re-run the tool')
    expect(bashDigest).toContain('older bash result elided')
    expect(bashDigest).toContain('safe/idempotent')
  })
})
