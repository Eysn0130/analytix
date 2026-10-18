// @vitest-environment jsdom
import { act, createElement, type ComponentProps } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { renderToStaticMarkup } from 'react-dom/server'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useSddDraftStore, type SddDraft } from '../../sdd/sdd-draft-store'
import { useWriteWorkspaceStore } from '../../write/write-workspace-store'
import { useChatStore } from '../../store/chat-store'
import type { WriteInlineCompletionRequest, WriteInlineCompletionResult } from '@shared/write-inline-completion'
import type { WriteRichEditor, WriteRichEditorHandle } from '../../write/tiptap/WriteRichEditor'
import type { WriteInlineAgent } from '../write/WriteInlineAgent'
import { SddDraftEditorView, SddAssistantToggleButton } from './SddDraftEditorView'

describe('SddDraftEditorView', () => {
  it('renders a control to reopen the Requirement AI panel', () => {
    const html = renderToStaticMarkup(
      createElement(SddAssistantToggleButton, {
        assistantOpen: false,
        onToggleAssistant: () => undefined,
        label: 'Requirement AI'
      })
    )

    expect(html).toContain('aria-label="Requirement AI"')
  })
})


const editor = vi.hoisted(() => ({ rich: true, apply: vi.fn() }))
vi.mock('react-i18next', async importOriginal => ({
  ...await importOriginal<typeof import('react-i18next')>(),
  useTranslation: () => ({ t: (key: string) => key })
}))
vi.mock('../../sdd/use-sdd-trace', () => ({ useSddTrace: () => null }))
vi.mock('../../sdd/sdd-draft-actions', () => ({ saveActiveSddDraftToDisk: vi.fn(async () => undefined), syncActiveSddDraftFromDisk: vi.fn() }))
vi.mock('../../store/chat-store', async () => {
  const { create } = await import('zustand')
  return { useChatStore: create(() => ({ activeThreadId: 'thread-a', workspaceRoot: '/workspace', threads: [], showTopNotice: vi.fn() })) }
})
vi.mock('../write/WriteMarkdownEditor', () => ({ WriteMarkdownEditor: () => null }))
vi.mock('../../write/tiptap/WriteRichEditor', async () => {
  const { useLayoutEffect } = await import('react')
  return { WriteRichEditor: (props: ComponentProps<typeof WriteRichEditor>) => {
    useLayoutEffect(() => {
      const handle = editor.rich ? {
        getProjectionText: () => props.value,
        applyProjectedReplacement: (range: { from: number; to: number }, original: string, replacement: string, instruction?: string) => {
          const applied = editor.apply(range, original, replacement, instruction)
          if (applied) props.onChange(props.value.slice(0, range.from) + replacement + props.value.slice(range.to))
          return applied
        }
      } as WriteRichEditorHandle : null
      props.handleRef!.current = handle
      return () => { props.handleRef!.current = null }
    }, [props.handleRef, props.value, editor.rich])
    return null
  } }
})
vi.mock('../write/WriteInlineAgent', () => ({ WriteInlineAgent: (props: ComponentProps<typeof WriteInlineAgent>) =>
  createElement('button', { 'data-inline-edit': true, 'data-busy': props.inFlight, onClick: () => props.onApplyEdit('Polish this') }, 'Apply') }))

const initialDraft = useSddDraftStore.getInitialState(), initialWrite = useWriteWorkspaceStore.getInitialState()
const content = '# Requirement\n\nOriginal sentence.\n'
const original = 'Original sentence.', from = content.indexOf(original), to = from + original.length
const draft: SddDraft = { id: 'draft-a', workspaceRoot: '/workspace', relativePath: '.analytixsdd/requirements/a/requirement.md',
  absolutePath: '/workspace/.analytixsdd/requirements/a/requirement.md', createdAt: '2026-10-17T00:00:00Z', updatedAt: '2026-10-17T00:00:00Z' }
const success: WriteInlineCompletionResult = { ok: true, completion: 'Rewritten sentence.', model: 'synthetic', mode: 'edit' }
let root: Root, host: HTMLDivElement, request: ReturnType<typeof vi.fn<(payload: WriteInlineCompletionRequest) => Promise<WriteInlineCompletionResult>>>, cancel: ReturnType<typeof vi.fn>
let mounted: boolean
const props = { leftSidebarCollapsed: false, assistantOpen: false, onToggleAssistant: () => undefined, onAssistantQuote: () => undefined,
  onPrototypeTurn: async () => false, onNext: () => undefined, onClose: () => undefined, nextDisabled: false }
const selection = () => ({ text: original, charCount: original.length, ranges: [{ from, to, text: original, charCount: original.length,
  startLine: 3, endLine: 3, startColumn: 1, endColumn: original.length }], anchorRect: { left: 10, right: 110, top: 20, bottom: 40, width: 100, height: 20 } })
const applyButton = () => host.querySelector<HTMLButtonElement>('[data-inline-edit]')!
const click = () => act(async () => applyButton().click())
async function renderEditor(rich = true) {
  editor.rich = rich
  await act(async () => root.render(createElement(SddDraftEditorView, props)))
  await act(async () => useWriteWorkspaceStore.getState().setSelection(selection()))
}
function pending() {
  let resolve!: (value: WriteInlineCompletionResult) => void, reject!: (error: Error) => void
  const promise = new Promise<WriteInlineCompletionResult>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
beforeEach(() => {
  vi.useFakeTimers(); vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  useSddDraftStore.setState({ ...initialDraft, activeDraft: draft, content, lastSavedContent: content }, true)
  useWriteWorkspaceStore.setState({ ...initialWrite, loadWriteSettings: vi.fn(async () => undefined) }, true)
  useChatStore.setState({ activeThreadId: 'thread-a', workspaceRoot: '/workspace', threads: [] })
  vi.mocked(useChatStore.getState().showTopNotice).mockClear()
  editor.rich = true; editor.apply.mockReset().mockReturnValue(true)
  request = vi.fn(async () => success); cancel = vi.fn(async () => ({ canceled: true }))
  vi.stubGlobal('analytix', { write: { requestWriteInlineCompletion: request, cancelWriteInlineCompletion: cancel } })
  host = document.createElement('div'); document.body.append(host); root = createRoot(host); mounted = true
})
afterEach(async () => {
  if (mounted) await act(async () => root.unmount())
  host.remove(); useSddDraftStore.setState(initialDraft, true); useWriteWorkspaceStore.setState(initialWrite, true)
  vi.clearAllTimers(); vi.useRealTimers(); vi.unstubAllGlobals(); localStorage.clear()
})

describe('SDD selection inline-edit ownership', () => {
  it.each([true, false])('binds the real request and applies in place (rich=%s)', async rich => {
    if (rich) request.mockResolvedValueOnce({ ...success, ok: true, action: { kind: 'edit', replacement: 'Rewritten sentence.', from, to, original } })
    await renderEditor(rich); await click()
    expect(request).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ threadId: 'thread-a', requestId: expect.stringMatching(/^[a-f0-9-]{36}$/),
      document: { path: draft.absolutePath }, currentFilePath: draft.absolutePath, workspaceRoot: '/workspace', mode: 'edit' }))
    expect(useSddDraftStore.getState().content).toBe(content.replace(original, 'Rewritten sentence.'))
    if (rich) expect(editor.apply).toHaveBeenCalledExactlyOnceWith({ from, to }, original, 'Rewritten sentence.', 'Polish this')
    else { expect(editor.apply).not.toHaveBeenCalled(); expect(useWriteWorkspaceStore.getState().recentEdits).toHaveLength(1) }
    expect(useChatStore.getState().showTopNotice).toHaveBeenCalledWith({ tone: 'success', message: 'writeInlineEditApplied' })
    expect(cancel).not.toHaveBeenCalled()
  })
  it('blocks same-tick duplicate requests', async () => {
    const wait = pending(); request.mockReturnValueOnce(wait.promise)
    await renderEditor(); await act(async () => { applyButton().click(); applyButton().click() })
    expect(request).toHaveBeenCalledTimes(1)
    await act(async () => wait.resolve(success))
    expect(editor.apply).toHaveBeenCalledTimes(1)
  })
  it.each(['document', 'document ABA', 'thread', 'thread ABA', 'selection', 'content', 'Escape', 'unmount'] as const)(
    'cancels on %s and ignores late success in both editor branches', async change => {
      for (const rich of [true, false]) {
        if (!mounted) { root = createRoot(host); mounted = true }
        await act(async () => {
          useSddDraftStore.setState({ activeDraft: draft, content, saveStatus: 'saved', operationStatus: 'idle', error: null })
          useChatStore.setState({ activeThreadId: 'thread-a' })
        })
        editor.apply.mockClear(); cancel.mockClear(); request.mockClear()
        const wait = pending(); request.mockReturnValueOnce(wait.promise)
        await renderEditor(rich); await click()
        const requestId = request.mock.calls[0][0].requestId
        await act(async () => {
          if (change === 'document' || change === 'document ABA') {
            useSddDraftStore.setState({ activeDraft: { ...draft, id: change === 'document ABA' ? 'draft-b' : draft.id, absolutePath: '/workspace/other.md' } })
            if (change === 'document ABA') useSddDraftStore.setState({ activeDraft: draft })
          } else if (change === 'thread' || change === 'thread ABA') {
            useChatStore.setState({ activeThreadId: 'thread-b' })
            if (change === 'thread ABA') useChatStore.setState({ activeThreadId: 'thread-a' })
          } else if (change === 'selection') useWriteWorkspaceStore.getState().setSelection({ text: '', charCount: 0, ranges: [] })
          else if (change === 'content') useSddDraftStore.getState().setContent(content + 'Typed later.')
          else if (change === 'Escape') document.body.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
          else { root.unmount(); mounted = false }
        })
        expect(cancel).toHaveBeenCalledExactlyOnceWith({ requestId })
        const before = useSddDraftStore.getState()
        vi.mocked(useChatStore.getState().showTopNotice).mockClear()
        await act(async () => wait.resolve(success))
        expect(editor.apply).not.toHaveBeenCalled()
        expect(useSddDraftStore.getState()).toBe(before)
        expect(useChatStore.getState().showTopNotice).not.toHaveBeenCalled()
      }
    })
  it('ignores a late rejection after cancellation and keeps a newer request busy', async () => {
    const old = pending(), fresh = pending(); request.mockReturnValueOnce(old.promise).mockReturnValueOnce(fresh.promise)
    await renderEditor(); await click()
    await act(async () => document.body.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })))
    await click()
    await act(async () => old.reject(new Error('synthetic old failure')))
    expect(useSddDraftStore.getState().operationStatus).toBe('idle')
    expect(applyButton().dataset.busy).toBe('true')
    await act(async () => fresh.resolve(success))
    expect(editor.apply).toHaveBeenCalledTimes(1)
    expect(request).toHaveBeenCalledTimes(2)
  })
  it.each(['thread', 'workspace', 'cancel-api'] as const)('does not send without a current %s', async missing => {
    await renderEditor()
    if (missing === 'thread') await act(async () => useChatStore.setState({ activeThreadId: null }))
    if (missing === 'workspace') await act(async () => useChatStore.setState({ workspaceRoot: '/other' }))
    if (missing === 'cancel-api') Reflect.deleteProperty(window.analytix.write, 'cancelWriteInlineCompletion')
    await click()
    expect(request).not.toHaveBeenCalled()
    expect(useSddDraftStore.getState().operationStatus).toBe('error')
  })
})
