import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ChatBlock } from '../agent/types'
import {
  notifySddChatTranscriptMirror,
  sddChatTranscriptRelativePath,
  serializeSddChatTranscript,
  writeSddChatTranscriptForThread
} from './sdd-chat-transcript'
import { markSddAssistantThread } from './sdd-thread-registry'
import {
  markPublicProjectionRevoked,
  resetPublicProjectionRevocationsForTests
} from '../lib/public-projection-revocation'

const UUID = '123e4567-e89b-12d3-a456-426614174000'
const DRAFT = `.analytixsdd/requirements/${UUID}/requirement.md`

function memoryStorage(): Storage {
  const values = new Map<string, string>()
  return {
    get length() { return values.size },
    clear: () => values.clear(),
    getItem: (key) => values.get(key) ?? null,
    key: (index) => [...values.keys()][index] ?? null,
    removeItem: (key) => { values.delete(key) },
    setItem: (key, value) => values.set(key, value)
  }
}

afterEach(() => {
  resetPublicProjectionRevocationsForTests()
  vi.unstubAllGlobals()
})

describe('sddChatTranscriptRelativePath', () => {
  it('builds the chat file path inside the requirement unit', () => {
    expect(sddChatTranscriptRelativePath(DRAFT, 'thr_abc-123')).toBe(
      `.analytixsdd/requirements/${UUID}/chat/thr_abc-123.md`
    )
  })

  it('refuses unsafe thread ids and non-unit drafts', () => {
    expect(sddChatTranscriptRelativePath(DRAFT, '../escape')).toBeNull()
    expect(sddChatTranscriptRelativePath(DRAFT, 'a/b')).toBeNull()
    expect(sddChatTranscriptRelativePath(DRAFT, '  ')).toBeNull()
    expect(sddChatTranscriptRelativePath('.analytixsdd/draft/x/requirement.md', 'thr_1')).toBeNull()
  })
})

describe('serializeSddChatTranscript', () => {
  it('prefers the human-entered displayText over composed prompts', () => {
    const hostileLegacyReasoning = {
      kind: 'reasoning',
      id: 'r1',
      text: '思考过程不应出现'
    } as unknown as ChatBlock
    const blocks: ChatBlock[] = [
      {
        kind: 'user',
        id: 'u1',
        text: 'Analytix is asking… full draft markdown inlined …',
        meta: { displayText: '帮我澄清需求' }
      },
      hostileLegacyReasoning,
      { kind: 'assistant', id: 'a1', text: '需要澄清三个问题。' },
      { kind: 'tool', id: 't1', summary: '读取 requirement.md', status: 'success' },
      { kind: 'user', id: 'u2', text: '直接输入的第二轮' },
      { kind: 'assistant', id: 'a2', text: '好的。' }
    ]
    const transcript = serializeSddChatTranscript(blocks, {
      threadId: 'thr_1',
      generatedAt: '2026-06-12T00:00:00.000Z'
    })

    expect(transcript).toContain('线程: thr_1')
    expect(transcript).toContain('## 用户\n\n帮我澄清需求')
    expect(transcript).not.toContain('full draft markdown')
    expect(transcript).not.toContain('思考过程不应出现')
    expect(transcript).toContain('## 需求 AI\n\n需要澄清三个问题。')
    expect(transcript).toContain('> [工具] Tool activity')
    expect(transcript).not.toContain('读取 requirement.md')
    expect(transcript).toContain('## 用户\n\n直接输入的第二轮')
    // One turn separator per user message.
    expect(transcript.match(/^---$/gm)).toHaveLength(2)
  })

  it('annotates non-success tool status', () => {
    const blocks: ChatBlock[] = [
      { kind: 'tool', id: 't1', summary: '写文件', status: 'error' }
    ]
    expect(serializeSddChatTranscript(blocks, { threadId: 'thr_1' })).toContain('> [工具] Tool activity（error）')
  })

  it('never serializes marker-free tool presentation or metadata text', () => {
    const marker = 'SENTINEL_MARKER_FREE_TOOL_TEXT_7E91'
    const blocks: ChatBlock[] = [{
      kind: 'tool',
      id: 't_marker',
      summary: marker,
      status: 'running',
      toolKind: 'command_execution',
      detail: `${marker}:detail`,
      filePath: `/tmp/${marker}`,
      meta: {
        command: `printf ${marker}`,
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          childId: 'child_1',
          childStatus: 'running',
          childLabel: marker
        }
      }
    }]

    const transcript = serializeSddChatTranscript(blocks, { threadId: 'thr_1' })

    expect(transcript).toContain('> [工具] Command activity（running）')
    expect(transcript).not.toContain(marker)
  })

  it('never exports complete, unterminated, or serialized provider reasoning', () => {
    const blocks: ChatBlock[] = [
      { kind: 'assistant', id: 'a1', text: '公开<think>PRIVATE_TAG</think>结论' },
      { kind: 'assistant', id: 'a2', text: '另一结论<think>PRIVATE_UNTERMINATED' },
      { kind: 'assistant', id: 'a3', text: '字段前\nreasoning_content: "PRIVATE_FIELD\nstill private"\n字段后' },
      { kind: 'assistant', id: 'a4', text: '干净的已接受结论' }
    ]
    const transcript = serializeSddChatTranscript(blocks, { threadId: 'thr_1' })
    expect(transcript).toContain('干净的已接受结论')
    expect(transcript).not.toContain('公开结论')
    expect(transcript).not.toContain('另一结论')
    expect(transcript).not.toContain('字段前')
    expect(transcript).not.toContain('字段后')
    expect(transcript).not.toMatch(/PRIVATE_|reasoning_content|<think>/)
  })

  it('never exports complete accounts from ordinary user or assistant text', () => {
    const account = '6222020202020202020'
    const transcript = serializeSddChatTranscript([
      {
        kind: 'user',
        id: 'u-account',
        text: `composed account=${account}`,
        meta: { displayText: `查询银行卡号 ${account}` }
      },
      { kind: 'assistant', id: 'a-account', text: `账号 ${account}` }
    ], { threadId: 'thr_account' })

    expect(transcript).not.toContain(account)
    expect(transcript).toContain('[ACCOUNT]')
  })
})

describe('SDD case transcript publication boundary', () => {
  it('never writes a case-bound transcript and removes an existing mirror', async () => {
    const localStorage = memoryStorage()
    const write = vi.fn(async () => ({ ok: true }))
    const deleteEntry = vi.fn(async () => ({ deleted: true }))
    vi.stubGlobal('window', {
      localStorage,
      analytix: { files: { write, deleteEntry } }
    })
    markSddAssistantThread({
      id: `/workspace:${DRAFT}`,
      workspaceRoot: '/workspace',
      relativePath: DRAFT
    } as any, 'thread-case', localStorage)

    await expect(writeSddChatTranscriptForThread({
      workspaceRoot: '/workspace',
      draftRelativePath: DRAFT,
      threadId: 'thread-case',
      blocks: [{ kind: 'assistant', id: 'private', text: 'PRIVATE_CASE_FACT' }],
      historyAuthority: 'case_boundary_only_v1'
    })).resolves.toBe(false)
    notifySddChatTranscriptMirror(() => ({
      activeThreadId: 'thread-case',
      blocks: [{ kind: 'assistant', id: 'private', text: 'PRIVATE_CASE_FACT' }],
      threads: [{
        id: 'thread-case',
        title: '案件分析',
        updatedAt: '2026-07-14T00:00:00.000Z',
        model: 'deepseek-chat',
        mode: 'agent',
        historyAuthority: 'case_boundary_only_v1'
      }]
    }))
    await Promise.resolve()

    expect(write).not.toHaveBeenCalled()
    expect(deleteEntry).toHaveBeenCalledWith({
      workspaceRoot: '/workspace',
      path: `.analytixsdd/requirements/${UUID}/chat/thread-case.md`
    })
  })

  it('compensates a transcript write that resolves after authority revocation', async () => {
    const localStorage = memoryStorage()
    let resolveWrite!: (value: { ok: boolean }) => void
    const transcriptWrite = new Promise<{ ok: boolean }>((resolve) => { resolveWrite = resolve })
    const write = vi.fn(() => transcriptWrite)
    const deleteEntry = vi.fn(async () => ({ deleted: true }))
    vi.stubGlobal('window', {
      localStorage,
      analytix: { files: { write, deleteEntry } }
    })
    markSddAssistantThread({
      id: `/workspace:${DRAFT}`,
      workspaceRoot: '/workspace',
      relativePath: DRAFT
    } as any, 'thread-late', localStorage)

    const pending = writeSddChatTranscriptForThread({
      workspaceRoot: '/workspace',
      draftRelativePath: DRAFT,
      threadId: 'thread-late',
      blocks: [{ kind: 'assistant', id: 'private', text: 'PRIVATE_LATE_FACT' }]
    })
    await Promise.resolve()
    markPublicProjectionRevoked('thread-late')
    resolveWrite({ ok: true })

    await expect(pending).resolves.toBe(false)
    expect(write).toHaveBeenCalledTimes(1)
    expect(deleteEntry).toHaveBeenCalledWith({
      workspaceRoot: '/workspace',
      path: `.analytixsdd/requirements/${UUID}/chat/thread-late.md`
    })
  })
})
