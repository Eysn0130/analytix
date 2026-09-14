// @vitest-environment jsdom
import { act, createElement, useRef } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { acceptChunk, getChunks, getOriginalDoc, rejectChunk } from '@codemirror/merge'
import { EditorView } from '@codemirror/view'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { WriteMarkdownEditor, type WriteMarkdownEditorHandle } from './WriteMarkdownEditor'
import { useWriteWorkspaceStore } from '../../write/write-workspace-store'

vi.mock('../../write/inline-completion', () => ({
  buildInlineCompletionExtension: () => [], buildInlineCompletionPayload: () => ({})
}))
vi.mock('../../write/markdown-live-preview', () => ({ writeMarkdownLivePreviewExtensions: () => [] }))

const workspaceRoot = '/synthetic/review'
const filePath = `${workspaceRoot}/draft.md`
const original = Array.from({ length: 60 }, (_, index) => `Unchanged document line ${index}`).join('\n')
const proposed = original.replace('line 2', 'line TWO').replace('line 25', 'line TWENTY-FIVE').replace('line 48', 'line FORTY-EIGHT')
let root: Root | null
let container: HTMLDivElement
let handle: { current: WriteMarkdownEditorHandle | null }
let write: ReturnType<typeof vi.fn>

function Harness() {
  const state = useWriteWorkspaceStore()
  const editorHandle = useRef<WriteMarkdownEditorHandle | null>(null)
  handle = editorHandle
  return createElement(WriteMarkdownEditor, {
    value: state.fileContent, workspaceRoot: state.workspaceRoot, filePath: state.activeFilePath,
    appearance: 'source', completionModel: '', completionEnabled: false,
    completionDebounceMs: 100, completionMinAcceptScore: 0,
    completionLongEnabled: false, completionLongDebounceMs: 100, completionLongMinAcceptScore: 0,
    onChange: state.setFileContent, onSelectionChange: () => undefined,
    onSaveShortcut: () => undefined, onReviewStateChange: state.setReviewActive,
    reviewRecovery: state.reviewActive ? state.reviewRecovery : null,
    onReviewSuspend: state.suspendReview, handleRef: editorHandle
  })
}

async function mount(): Promise<void> {
  root = createRoot(container)
  await act(async () => root!.render(createElement(Harness)))
}
async function unmount(): Promise<void> {
  await act(async () => root?.unmount())
  root = null
}
function editor(): EditorView {
  const dom = container.querySelector('.cm-content')
  const view = dom && EditorView.findFromDOM(dom as HTMLElement)
  if (!view) throw new Error('Editor did not mount')
  return view
}

beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  write = vi.fn(async () => ({ ok: true, path: filePath, savedAt: 'synthetic' }))
  Object.defineProperty(window, 'analytix', { configurable: true, value: { files: { write } } })
  useWriteWorkspaceStore.getState().resetWorkspace()
  useWriteWorkspaceStore.setState({ workspaceRoot, activeFilePath: filePath,
    activeFileKind: 'text', fileContent: original, saveStatus: 'saved' })
  container = document.createElement('div')
  document.body.append(container)
})
afterEach(async () => {
  await unmount()
  container.remove()
  useWriteWorkspaceStore.getState().resetWorkspace()
  Reflect.deleteProperty(window, 'analytix')
  vi.unstubAllGlobals()
})

describe('document review across editor unmounts', () => {
  it.each(['accept', 'reject'] as const)('retains partial decisions then resolves remaining chunks with %s', async (resolution) => {
    await mount()
    await act(async () => { expect(handle.current!.beginDiffReview({ original, nextDoc: proposed })).toBe(true) })
    expect(getChunks(editor().state)!.chunks).toHaveLength(3)
    await act(async () => { acceptChunk(editor(), getChunks(editor().state)!.chunks[0].fromB) })
    await act(async () => { rejectChunk(editor(), getChunks(editor().state)!.chunks[0].fromB) })
    const resolvedOriginal = getOriginalDoc(editor().state).toString()
    const resolvedNext = editor().state.doc.toString()
    expect(getChunks(editor().state)!.chunks).toHaveLength(1)
    await unmount()
    expect(useWriteWorkspaceStore.getState()).toMatchObject({
      reviewActive: true, fileContent: original,
      reviewRecovery: { workspaceRoot, filePath, baseline: original, original: resolvedOriginal, nextDoc: resolvedNext }
    })
    expect(await useWriteWorkspaceStore.getState().flushSave(workspaceRoot)).toBe(false)
    expect(write).not.toHaveBeenCalled()
    await mount()
    expect(getOriginalDoc(editor().state).toString()).toBe(resolvedOriginal)
    expect(editor().state.doc.toString()).toBe(resolvedNext)
    expect(getChunks(editor().state)!.chunks).toHaveLength(1)
    await act(async () => {
      if (resolution === 'accept') handle.current!.acceptAllDiff()
      else handle.current!.rejectAllDiff()
    })
    expect(useWriteWorkspaceStore.getState()).toMatchObject({
      reviewActive: false, reviewRecovery: null,
      fileContent: resolution === 'accept' ? resolvedNext : resolvedOriginal
    })
    await act(async () => { expect(await useWriteWorkspaceStore.getState().flushSave(workspaceRoot)).toBe(true) })
    expect(write).toHaveBeenCalledExactlyOnceWith({ workspaceRoot, path: filePath,
      content: resolution === 'accept' ? resolvedNext : resolvedOriginal })
  })

  it('does not suspend or restore a review into another document or working copy', async () => {
    await mount()
    await act(async () => { handle.current!.beginDiffReview({ original, nextDoc: proposed }) })
    await unmount()
    const recovery = useWriteWorkspaceStore.getState().reviewRecovery!
    useWriteWorkspaceStore.setState({ workspaceRoot: '/synthetic/other', activeFilePath: '/synthetic/other/draft.md',
      fileContent: 'Other document', reviewActive: false })
    useWriteWorkspaceStore.getState().suspendReview(recovery)
    await mount()
    expect(handle.current!.isDiffReviewActive()).toBe(false)
    expect(editor().state.doc.toString()).toBe('Other document')
    await unmount()
    useWriteWorkspaceStore.setState({ workspaceRoot, activeFilePath: filePath, fileContent: 'New working copy', reviewActive: false })
    useWriteWorkspaceStore.getState().suspendReview(recovery)
    await mount()
    expect(handle.current!.isDiffReviewActive()).toBe(false)
    expect(editor().state.doc.toString()).toBe('New working copy')
    expect(write).not.toHaveBeenCalled()
  })
})
