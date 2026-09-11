import { describe, expect, it } from 'vitest'
import type { ChatBlock, NormalizedThread } from '../agent/types'
import { chatBlockFromItem } from '../agent/analytix-mapper'
import { buildThreadMarkdown } from './thread-markdown'

describe('buildThreadMarkdown', () => {
  it('does not export reasoning blocks, think markup, or legacy reasoning fields', () => {
    const thread = {
      id: 'thread-1',
      title: 'Safe export',
      updatedAt: '2026-07-10T00:00:00Z'
    } as NormalizedThread
    const hostileLegacyReasoning = {
      kind: 'reasoning',
      id: 'reasoning',
      text: 'PRIVATE_REASONING_SENTINEL'
    } as unknown as ChatBlock
    const blocks: ChatBlock[] = [
      hostileLegacyReasoning,
      { kind: 'assistant', id: 'answer', text: '<think>INLINE_REASONING_SENTINEL</think>public answer' },
      { kind: 'assistant', id: 'clean-answer', text: 'clean public answer' },
      {
        kind: 'tool',
        id: 'tool',
        summary: 'tool result',
        status: 'success',
        toolKind: 'tool_call',
        detail: 'reasoningContent: TOOL_REASONING_SENTINEL\npublic detail'
      },
      {
        kind: 'compaction',
        id: 'compaction',
        summary: 'assistant_reasoning: COMPACTION_REASONING_SENTINEL\npublic summary',
        status: 'success'
      }
    ]

    const markdown = buildThreadMarkdown(thread, '/workspace', blocks)
    expect(markdown).toContain('clean public answer')
    expect(markdown).not.toContain('public detail')
    expect(markdown).not.toContain('public summary')
    expect(markdown).not.toMatch(/PRIVATE_REASONING|INLINE_REASONING|TOOL_REASONING|COMPACTION_REASONING/)
    expect(markdown).not.toMatch(/assistant_reasoning|reasoningContent|<think>/)
  })

  it('drops multiline and unterminated private reasoning field values', () => {
    const thread = {
      id: 'thread-2',
      title: 'Safe multiline export',
      updatedAt: '2026-07-10T00:00:00Z'
    } as NormalizedThread
    const blocks: ChatBlock[] = [
      {
        kind: 'tool',
        id: 'tool-multiline',
        summary: 'tool result',
        status: 'success',
        toolKind: 'tool_call',
        detail: '{"reasoning_content":"MULTILINE_PRIVATE_START\nMULTILINE_PRIVATE_END","public":"kept"}'
      },
      {
        kind: 'assistant',
        id: 'assistant-unterminated',
        text: 'public prefix\nassistant_reasoning = "UNTERMINATED_PRIVATE\nstill private'
      }
    ]

    const markdown = buildThreadMarkdown(thread, '/workspace', blocks)
    expect(markdown).not.toContain('"public":"kept"')
    expect(markdown).not.toContain('public prefix')
    expect(markdown).not.toMatch(/MULTILINE_PRIVATE|UNTERMINATED_PRIVATE|still private/)
  })

  it('does not export hostile legacy tool-result payload', () => {
    const sentinel = 'PRIVATE_TOOL_EXPORT_SENTINEL'
    const thread = {
      id: 'thread-tool-export',
      title: 'Safe tool export',
      updatedAt: '2026-07-14T00:00:00Z'
    } as NormalizedThread
    const block = chatBlockFromItem({
      id: 'item-tool-export',
      turnId: 'turn-tool-export',
      threadId: thread.id,
      role: 'tool',
      status: 'completed',
      createdAt: '2026-07-14T00:00:00.000Z',
      kind: 'tool_result',
      toolName: 'mcp__hostile__query',
      callId: 'call-tool-export',
      output: {
        account: '6222020202020202020',
        content: sentinel,
        dataUrl: `data:image/png;base64,${sentinel}`,
        citations: [{ sourceId: sentinel }]
      }
    })
    const markdown = buildThreadMarkdown(thread, '/workspace', block ? [block] : [])
    expect(markdown).toContain('mcp__hostile__query')
    expect(markdown).not.toContain(sentinel)
    expect(markdown).not.toContain('6222020202020202020')
    expect(markdown).not.toContain('data:image')
  })

  it('projects complete accounts from every ordinary markdown text surface', () => {
    const account = '6222020202020202020'
    const thread = {
      id: 'thread-account-export',
      title: `案件账号 ${account}`,
      workspace: `/workspace/${account}`,
      updatedAt: '2026-07-14T00:00:00Z'
    } as NormalizedThread
    const blocks: ChatBlock[] = [
      { kind: 'user', id: 'user-account', text: `查询银行卡号 ${account}` },
      { kind: 'assistant', id: 'assistant-account', text: `账号 ${account}` },
      { kind: 'system', id: 'system-account', text: `account=${account}` }
    ]

    const markdown = buildThreadMarkdown(thread, `/fallback/${account}`, blocks)
    expect(markdown).not.toContain(account)
    expect(markdown.match(/\[ACCOUNT\]/g)?.length).toBeGreaterThanOrEqual(4)
  })
})
