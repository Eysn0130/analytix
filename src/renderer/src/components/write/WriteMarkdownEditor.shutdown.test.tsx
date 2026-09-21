// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { EditorView } from '@codemirror/view'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { WriteMarkdownEditor } from './WriteMarkdownEditor'
import { createWriteShutdownHandler } from '../../write/write-shutdown'
import { useWriteWorkspaceStore } from '../../write/write-workspace-store'
import { useChatStore } from '../../store/chat-store'
import type { buildInlineCompletionExtension, InlineCompletionRequestContext } from '../../write/inline-completion'
const completion = vi.hoisted(() => ({ options: undefined as Parameters<typeof buildInlineCompletionExtension>[0] | undefined }))
vi.mock('../../store/chat-store', async () => {
  const { createStore } = await import('zustand/vanilla')
  return { useChatStore: createStore(() => ({ activeThreadId: 'thread-a' })) }
})
vi.mock('../../write/inline-completion',async importOriginal=>{
  const original = await importOriginal<typeof import('../../write/inline-completion')>()
  return { ...original,
    buildInlineCompletionExtension:(options: Parameters<typeof buildInlineCompletionExtension>[0])=>{ completion.options=options; return original.buildInlineCompletionExtension(options) },
    buildInlineCompletionPayload:(_context: unknown, options: unknown)=>options
  }
})
vi.mock('../../write/markdown-live-preview',()=>({writeMarkdownLivePreviewExtensions:()=>[]}))
let root:Root, container:HTMLDivElement, handler:ReturnType<typeof createWriteShutdownHandler>
const requestId='00000000-0000-4000-8000-000000000001'
const view=()=>EditorView.findFromDOM(container.querySelector('.cm-content') as HTMLElement)!
beforeEach(async()=>{
  useChatStore.setState({ activeThreadId: 'thread-a' })
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true)
  const range=document.createRange.bind(document)
  vi.spyOn(document,'createRange').mockImplementation(()=>Object.assign(range(),{getClientRects:()=>[],getBoundingClientRect:()=>new DOMRect()}))
  useWriteWorkspaceStore.getState().resetWorkspace();handler=createWriteShutdownHandler()
  container=document.createElement('div');document.body.append(container);root=createRoot(container)
  await act(async()=>root.render(createElement(WriteMarkdownEditor,{value:'最后一键',appearance:'source',completionEnabled:false,completionModel:'',completionDebounceMs:100,completionMinAcceptScore:0,
    completionLongEnabled:false,completionLongDebounceMs:100,completionLongMinAcceptScore:0,
    onChange:useWriteWorkspaceStore.getState().setFileContent,onSelectionChange:()=>undefined,onSaveShortcut:()=>undefined})))
})
afterEach(async()=>{
  await act(async()=>{await handler({requestId,phase:'cancel'});root.unmount()})
  container.remove();useWriteWorkspaceStore.getState().resetWorkspace();vi.useRealTimers();vi.restoreAllMocks();vi.unstubAllGlobals()
})
test('freezes the live CodeMirror instance synchronously, blocks delayed transactions, then restores editability',async()=>{
  await act(async()=>{expect(await handler({requestId,phase:'prepare'})).toEqual({result:'ready'})})
  expect(view().contentDOM.getAttribute('contenteditable')).toBe('false')
  await act(async()=>view().dispatch({changes:{from:0,insert:'late completion'}}))
  expect(view().state.doc.toString()).toBe('最后一键')
  await act(async()=>{await handler({requestId,phase:'cancel'})})
  expect(view().contentDOM.getAttribute('contenteditable')).toBe('true')
  await act(async()=>view().dispatch({changes:{from:0,insert:'恢复'}}))
  expect(view().state.doc.toString()).toBe('恢复最后一键')
})
test('active CodeMirror composition blocks close without changing DOM editability or blurring',async()=>{
  vi.spyOn(view(),'composing','get').mockReturnValue(true)
  const blur=vi.spyOn(view().contentDOM,'blur')
  expect(await handler({requestId,phase:'prepare'})).toEqual({result:'blocked',reason:'composing'})
  expect(blur).not.toHaveBeenCalled();expect(view().contentDOM.getAttribute('contenteditable')).toBe('true')
  expect(view().state.doc.toString()).toBe('最后一键')
})
test('binds completion to the document owner and drops responses after a thread round trip',async()=>{
  const request = vi.fn().mockResolvedValue({ ok: true, completion: 'next' })
  Object.defineProperty(window, 'analytix', { configurable: true, value: { write: { requestWriteInlineCompletion: request } } })
  const context = { filePath: '' } as InlineCompletionRequestContext
  expect(await completion.options!.requestCompletion(context, 'short')).toMatchObject({ text: 'next' })
  expect(request).toHaveBeenCalledWith(expect.objectContaining({ threadId: 'thread-a' }))
  useChatStore.setState({ activeThreadId: 'thread-b' })
  expect(await completion.options!.requestCompletion(context, 'short')).toBeNull()
  useChatStore.setState({ activeThreadId: null })
  expect(await completion.options!.requestCompletion(context, 'short')).toBeNull()
  expect(request).toHaveBeenCalledTimes(1)
  useChatStore.setState({ activeThreadId: 'thread-a' })
  let resolve!: (value: unknown) => void
  request.mockImplementationOnce(() => new Promise(done => { resolve = done }))
  const pending = completion.options!.requestCompletion(context, 'short')
  useChatStore.setState({ activeThreadId: 'thread-b' })
  useChatStore.setState({ activeThreadId: 'thread-a' })
  resolve({ ok: true, completion: 'late' })
  expect(await pending).toBeNull()
})
test('removes an already displayed CodeMirror ghost immediately when its task loses ownership',async()=>{
  vi.useFakeTimers()
  const request = vi.fn().mockResolvedValue({ ok: true, completion: 'a focused continuation' })
  Object.defineProperty(window, 'analytix', { configurable: true, value: { write: { requestWriteInlineCompletion: request } } })
  await act(async()=>root.render(createElement(WriteMarkdownEditor,{value:'This is ',appearance:'source',completionEnabled:true,completionModel:'',completionDebounceMs:10,completionMinAcceptScore:0,
    completionLongEnabled:false,completionLongDebounceMs:100,completionLongMinAcceptScore:0,
    onChange:useWriteWorkspaceStore.getState().setFileContent,onSelectionChange:()=>undefined,onSaveShortcut:()=>undefined})))
  await act(async()=>view().dispatch({selection:{anchor:8}}))
  await act(async()=>{await vi.advanceTimersByTimeAsync(1000)})
  expect(container.querySelector('.cm-inline-completion')?.textContent).toContain('a focused continuation')
  await act(async()=>useChatStore.setState({activeThreadId:'thread-b'}))
  expect(container.querySelector('.cm-inline-completion')).toBeNull()
  await act(async()=>{view().contentDOM.dispatchEvent(new KeyboardEvent('keydown',{key:'Tab',bubbles:true,cancelable:true}))})
  expect(view().state.doc.toString()).not.toContain('a focused continuation')
})
