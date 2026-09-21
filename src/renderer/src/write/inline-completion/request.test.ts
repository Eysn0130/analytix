// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { WriteInlineCompletionRequest, WriteInlineCompletionResult } from '@shared/write-inline-completion'
import { useWriteWorkspaceStore } from '../write-workspace-store'
import { requestDocumentInlineCompletion } from './request'

const payload: WriteInlineCompletionRequest = {
  threadId: 'thread-a', workspaceRoot: '/workspace', currentFilePath: '/workspace/plan.md',
  prefix: 'This is ', suffix: '', cursor: { line: 1, column: 9 },
  context: { language: 'markdown', currentLinePrefix: 'This is ', currentLineSuffix: '', previousLine: '',
    previousNonEmptyLine: '', nextLine: '', indentation: '', signals: { list: false, quote: false,
      heading: false, table: false, atLineEnd: true, endsWithSentencePunctuation: false,
      previousLineEndsWithSentencePunctuation: false, prefersNewLineCompletion: false, paragraphBreakOpportunity: false } },
  policy: { name: 'inline', instruction: 'Continue', acceptanceCriteria: [], rejectionCriteria: [] },
  preview: { local: 'This is ', documentTail: 'This is ' }
}
const success: WriteInlineCompletionResult = { ok: true, completion: 'a useful continuation', model: 'synthetic', mode: 'short' }
const session = { sessionId: 'session-a', objectId: 'a'.repeat(64), revision: 'b'.repeat(64) }
const initial = useWriteWorkspaceStore.getInitialState()
let request: ReturnType<typeof vi.fn<(request: WriteInlineCompletionRequest) => Promise<WriteInlineCompletionResult>>>
let cancel: ReturnType<typeof vi.fn<(request: { requestId: string }) => Promise<void>>>
function deferred() {
  let resolve!: (result: WriteInlineCompletionResult) => void
  const promise = new Promise<WriteInlineCompletionResult>(done => { resolve = done })
  return { promise, resolve }
}
function openSession() {
  useWriteWorkspaceStore.setState({ workspaceRoot: payload.workspaceRoot!, activeFilePath: payload.currentFilePath!, objectSession: session })
}
beforeEach(() => {
  useWriteWorkspaceStore.setState(initial, true)
  request = vi.fn(async () => success)
  cancel = vi.fn(async () => undefined)
  Object.defineProperty(window, 'analytix', { configurable: true,
    value: { write: { requestWriteInlineCompletion: request, cancelWriteInlineCompletion: cancel } } })
})
afterEach(() => {
  vi.restoreAllMocks()
  useWriteWorkspaceStore.setState(initial, true)
  Reflect.deleteProperty(window, 'analytix')
})

describe('document inline request ownership', () => {
  it('keeps SDD/Plan path requests usable without a workspace session', async () => {
    const pending = deferred()
    request.mockReturnValueOnce(pending.promise)
    const result = requestDocumentInlineCompletion(payload, new AbortController().signal)
    useWriteWorkspaceStore.setState({ workspaceRoot: '/other', activeFilePath: '/other/unrelated.md', objectSession: session })
    pending.resolve(success)
    expect(await result).toEqual(success)
    expect(request).toHaveBeenCalledWith(expect.objectContaining({ threadId: 'thread-a', document: { path: payload.currentFilePath }, requestId: expect.any(String) }))
    expect(cancel).not.toHaveBeenCalled()
  })

  it('uses the matching session and never retries rejection through a path', async () => {
    openSession()
    request.mockResolvedValueOnce({ ok: false, message: 'session rejected' })
    expect(await requestDocumentInlineCompletion(payload, new AbortController().signal)).toEqual({ ok: false, message: 'session rejected' })
    expect(request).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ document: {
      sessionId: session.sessionId, objectId: session.objectId, baseRevision: session.revision
    } }))
  })

  it.each(['revision', 'session', 'file', 'workspace', 'shutdown'] as const)('irreversibly revokes a session on %s changes, including ABA', async (change) => {
    openSession()
    const pending = deferred()
    request.mockReturnValueOnce(pending.promise)
    const controller = new AbortController()
    const result = requestDocumentInlineCompletion(payload, controller.signal)
    const before = useWriteWorkspaceStore.getState()
    switch (change) {
      case 'revision': useWriteWorkspaceStore.setState({ objectSession: { ...session, revision: 'c'.repeat(64) } }); break
      case 'session': useWriteWorkspaceStore.setState({ objectSession: null }); break
      case 'file': useWriteWorkspaceStore.setState({ activeFilePath: '/workspace/other.md' }); break
      case 'workspace': useWriteWorkspaceStore.setState({ workspaceRoot: '/other' }); break
      case 'shutdown': useWriteWorkspaceStore.setState({ shutdownFrozen: true }); break
    }
    useWriteWorkspaceStore.setState(before, true)
    pending.resolve(success)
    expect(await result).toMatchObject({ ok: false })
    controller.abort()
    expect(cancel).toHaveBeenCalledExactlyOnceWith({ requestId: request.mock.calls[0][0].requestId })
  })

  it('cancels only the aborted request and keeps the other request active', async () => {
    const first = deferred(), second = deferred()
    request.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
    const controller = new AbortController()
    const firstResult = requestDocumentInlineCompletion(payload, controller.signal)
    const secondResult = requestDocumentInlineCompletion(payload, new AbortController().signal)
    controller.abort()
    const [firstId, secondId] = request.mock.calls.map(([input]) => input.requestId)
    expect(firstId).not.toBe(secondId)
    expect(cancel).toHaveBeenCalledExactlyOnceWith({ requestId: firstId })
    first.resolve(success); second.resolve(success)
    expect(await firstResult).toMatchObject({ ok: false })
    expect(await secondResult).toEqual(success)
  })

  it.each([false, true])('removes its listener and store subscription when rejection=%s', async (reject) => {
    openSession()
    const subscribe = useWriteWorkspaceStore.subscribe
    const unsubscribe = vi.fn()
    vi.spyOn(useWriteWorkspaceStore, 'subscribe').mockImplementation(listener => {
      const remove = subscribe(listener)
      return () => { unsubscribe(); remove() }
    })
    const controller = new AbortController()
    const remove = vi.spyOn(controller.signal, 'removeEventListener')
    if (reject) request.mockRejectedValueOnce(new Error('synthetic failure'))
    const result = requestDocumentInlineCompletion(payload, controller.signal)
    if (reject) await expect(result).rejects.toThrow('synthetic failure')
    else expect(await result).toEqual(success)
    expect(unsubscribe).toHaveBeenCalledOnce()
    expect(remove).toHaveBeenCalledWith('abort', expect.any(Function))
    controller.abort()
    useWriteWorkspaceStore.setState({ objectSession: null })
    expect(cancel).not.toHaveBeenCalled()
  })

  it.each(['cancel-api', 'thread', 'path', 'aborted'])('fails closed before calling transport without %s', async missing => {
    const controller = new AbortController()
    const input = { ...payload }
    if (missing === 'cancel-api') Reflect.deleteProperty(window.analytix.write, 'cancelWriteInlineCompletion')
    if (missing === 'thread') input.threadId = undefined
    if (missing === 'path') input.currentFilePath = undefined
    if (missing === 'aborted') controller.abort()
    expect(await requestDocumentInlineCompletion(input, controller.signal)).toMatchObject({ ok: false })
    expect(request).not.toHaveBeenCalled()
  })
})
