import { describe, expect, it, vi } from 'vitest'
import { prepareWorkbenchDocumentMessage, workbenchEmptyMessageKeys } from './workbench-document-message'
import { parseWritePromptForDisplay } from './quoted-selection'

const quote = {
  id: 'quote-alpha', text: '中文工作副本🙂', sourceTitle: 'report.md',
  sourceFilePath: '/workspace/report.md', charCount: 7, createdAt: '2026-09-14',
  workspaceRoot: '/workspace', snapshotContent: '# draft\n中文工作副本🙂'
}
const base = {
  threadId: 'thread-a',
  input: '今天几点？', quotes: [], editorRequest: false,
  workspaceRoot: '/workspace', activeFilePath: '/workspace/report.md',
  requestUserInputAvailable: true
}

describe('unified conversation document context', () => {
  it('keeps quote-only request and bubble labels in document semantics', async () => {
    const keys = workbenchEmptyMessageKeys(0, 1, 0)
    expect(keys).toEqual({ prompt: 'composerFileOnlyPrompt', display: 'composerFileOnlyDisplay', count: 1 })
    const text = await prepareWorkbenchDocumentMessage({ ...base, input: '请查看引用内容', quotes: [quote] })
    expect(parseWritePromptForDisplay(text)).toMatchObject({ userInput: '请查看引用内容', quotes: [{ text: quote.text }] })
    expect(workbenchEmptyMessageKeys(1, 0, 0).display).toBe('composerFileOnlyDisplay')
    expect(workbenchEmptyMessageKeys(0, 1, 1).display).toBe('composerFileAndImageOnlyDisplay')
    expect(workbenchEmptyMessageKeys(0, 0, 1).display).toBe('composerImageOnlyDisplay')
  })
  it('leaves ordinary chat unchanged with an open file, without retrieval or a writing persona', async () => {
    const retrieveContext = vi.fn()
    expect(await prepareWorkbenchDocumentMessage({ ...base, editorPersona: 'EXPLICIT_EDITOR_PRESET', retrieveContext })).toBe(base.input)
    expect(retrieveContext).not.toHaveBeenCalled()
  })

  it('uses the explicit working-copy quote without retrieving the disk file or neighboring content', async () => {
    const retrieveContext = vi.fn()
    const text = await prepareWorkbenchDocumentMessage({ ...base, input: '解释这段', quotes: [quote], editorPersona: 'EXPLICIT_EDITOR_PRESET', retrieveContext })
    expect(retrieveContext).not.toHaveBeenCalled()
    expect(text).not.toContain('当前写作 Agent 人设')
    expect(text).not.toContain('EXPLICIT_EDITOR_PRESET')
    expect(text).not.toContain('当前文件:')
    expect(parseWritePromptForDisplay(text)).toMatchObject({
      userInput: '解释这段', quotes: [{ sourceTitle: 'report.md', text: quote.text }]
    })
  })

  it('retains the explicit editor retrieval path and its current input capability', async () => {
    const retrieveContext = vi.fn().mockResolvedValue({ ok: true, context: null })
    const text = await prepareWorkbenchDocumentMessage({ ...base, editorRequest: true, editorPersona: 'EXPLICIT_EDITOR_PRESET', requestUserInputAvailable: false, retrieveContext })
    expect(retrieveContext).toHaveBeenCalledOnce()
    expect(retrieveContext).toHaveBeenCalledWith(expect.objectContaining({ threadId: 'thread-a', currentFilePath: '/workspace/report.md' }))
    expect(text).toContain('当前文件: report.md')
    expect(text).toContain('EXPLICIT_EDITOR_PRESET')
    expect(text).toContain('普通文本')
    expect(text).not.toContain('所以请放心直接改')
  })

  it('does not lose the quote when optional retrieval fails', async () => {
    const text = await prepareWorkbenchDocumentMessage({ ...base, quotes: [quote], editorRequest: true, retrieveContext: vi.fn().mockRejectedValue(new Error('unavailable')) })
    expect(parseWritePromptForDisplay(text)?.quotes[0]?.text).toBe(quote.text)
  })
})
