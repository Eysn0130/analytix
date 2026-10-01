import { describe, expect, it } from 'vitest'
import { composeWritePrompt } from './quoted-selection'
import { workbenchReferencesCurrent, type WorkbenchReferenceSnapshot } from './workbench-reference-snapshot'

function reference(): WorkbenchReferenceSnapshot {
  return { threadId: 'conversation-a', threadWorkspace: '/synthetic/workspace',
    documentWorkspace: '/synthetic/workspace', filePath: '/synthetic/workspace/report.md',
    content: '尚未保存的草稿\n选中段落', quotes: [{ id: 'quote-a', text: '选中段落',
      workspaceRoot: '/synthetic/workspace', sourceFilePath: '/synthetic/workspace/report.md',
      sourceTitle: 'report.md', snapshotContent: '尚未保存的草稿\n选中段落',
      charCount: 4, createdAt: '2026-09-14T00:00:00Z', lineStart: 2, lineEnd: 2 }] }
}

describe('working-copy conversation references', () => {
  it('uses the frozen selection without exposing the unrelated draft snapshot in the prompt', () => {
    const snapshot = reference()
    expect(workbenchReferencesCurrent(snapshot, snapshot)).toBe(true)
    const prompt = composeWritePrompt('解释选区', [...snapshot.quotes])
    expect(prompt).toContain('选中段落')
    expect(prompt).not.toContain('尚未保存的草稿')
    expect(prompt).not.toContain('snapshotContent')
  })
  it.each([
    { threadId: 'conversation-b' }, { threadWorkspace: '/other' },
    { documentWorkspace: '/other' }, { filePath: '/synthetic/workspace/other.md' },
    { content: 'AI等待时的新编辑' }, { filePath: null }
  ])('refuses a changed conversation, scope, file or draft: %o', (change) => {
    const snapshot = reference()
    expect(workbenchReferencesCurrent(snapshot, { ...snapshot, ...change })).toBe(false)
  })
  it('rejects references without the current local binding', () => {
    const snapshot = reference()
    const quote = { ...snapshot.quotes[0], workspaceRoot: undefined }
    expect(workbenchReferencesCurrent({ ...snapshot, quotes: [quote] }, snapshot)).toBe(false)
  })
  it('fences document actions without a selection while allowing ordinary chat to ignore document changes', () => {
    const snapshot = { ...reference(), quotes: [] }
    const changed = { ...snapshot, content: 'changed during retrieval' }
    expect(workbenchReferencesCurrent(snapshot, changed)).toBe(true)
    expect(workbenchReferencesCurrent({ ...snapshot, documentContext: true }, changed)).toBe(false)
  })
})
