// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import type { Editor as TiptapEditor } from '@tiptap/core'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { WriteRichEditor } from './WriteRichEditor'
import { createWriteShutdownHandler } from '../write-shutdown'
import { useWriteWorkspaceStore } from '../write-workspace-store'
import { useChatStore } from '../../store/chat-store'
import type { InlineCompletionRequestContext } from '../inline-completion'
import type { WriteRichInlineCompletionOptions } from './extensions/inline-completion'
import { writeRichInlineCompletionTestInternals } from './extensions/inline-completion'
vi.mock('../../store/chat-store', async () => {
  const { createStore } = await import('zustand/vanilla')
  return { useChatStore: createStore(() => ({ activeThreadId: 'thread-a' })) }
})
const capture=vi.hoisted(()=>({editor:undefined as TiptapEditor|undefined}))
vi.mock('@tiptap/core',async importOriginal=>{
  const core=await importOriginal<typeof import('@tiptap/core')>()
  return {...core,Editor:class extends core.Editor{constructor(options:any){super(options);capture.editor=this}}}
})
let root:Root, container:HTMLDivElement, handler:ReturnType<typeof createWriteShutdownHandler>
const requestId='00000000-0000-4000-8000-000000000002'
beforeEach(async()=>{
  useChatStore.setState({ activeThreadId: 'thread-a' })
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true)
  useWriteWorkspaceStore.getState().resetWorkspace();handler=createWriteShutdownHandler()
  container=document.createElement('div');document.body.append(container);root=createRoot(container)
  await act(async()=>root.render(createElement(WriteRichEditor,{value:'最后一键',completionEnabled:false,fallback:null,
    onChange:useWriteWorkspaceStore.getState().setFileContent,onSelectionChange:()=>undefined,onSaveShortcut:()=>undefined})))
  expect(capture.editor).toBeDefined()
})
afterEach(async()=>{
  await act(async()=>{await handler({requestId,phase:'cancel'});root.unmount()})
  container.remove();useWriteWorkspaceStore.getState().resetWorkspace();vi.restoreAllMocks();vi.unstubAllGlobals()
})
test('freezes actual ProseMirror editing and late commands, then restores the rich editor',async()=>{
  const editor=capture.editor!
  await act(async()=>{expect(await handler({requestId,phase:'prepare'})).toEqual({result:'ready'})})
  expect(editor.isEditable).toBe(false)
  await act(async()=>{editor.commands.insertContent('late completion')})
  expect(editor.getText()).toBe('最后一键')
  await act(async()=>{await handler({requestId,phase:'cancel'})})
  expect(editor.isEditable).toBe(true)
  await act(async()=>{editor.commands.insertContent('恢复')})
  expect(editor.getText()).toContain('恢复')
})
test('active rich composition preserves the editable document and cancels preparation',async()=>{
  const editor=capture.editor!;vi.spyOn(editor.view,'composing','get').mockReturnValue(true)
  const blur=vi.spyOn(editor.view.dom,'blur')
  expect(await handler({requestId,phase:'prepare'})).toEqual({result:'blocked',reason:'composing'})
  expect(blur).not.toHaveBeenCalled();expect(editor.isEditable).toBe(true);expect(editor.getText()).toBe('最后一键')
})
test('binds rich completion to the document owner and rejects a late thread round trip',async()=>{
  const options = capture.editor!.extensionManager.extensions.find(extension => extension.name === 'writeRichInlineCompletion')!.options as WriteRichInlineCompletionOptions
  const request = vi.fn().mockResolvedValue({ ok: true, completion: 'next' })
  Object.defineProperty(window, 'analytix', { configurable: true, value: { write: { requestWriteInlineCompletion: request } } })
  const context = { filePath: '' } as InlineCompletionRequestContext
  expect(await options.requestCompletion(context, 'short')).toMatchObject({ text: 'next' })
  expect(request).toHaveBeenCalledWith(expect.objectContaining({ threadId: 'thread-a' }))
  useChatStore.setState({ activeThreadId: 'thread-b' })
  expect(await options.requestCompletion(context, 'short')).toBeNull()
  useChatStore.setState({ activeThreadId: null })
  expect(await options.requestCompletion(context, 'short')).toBeNull()
  expect(request).toHaveBeenCalledTimes(1)
  useChatStore.setState({ activeThreadId: 'thread-a' })
  let resolve!: (value: unknown) => void
  request.mockImplementationOnce(() => new Promise(done => { resolve = done }))
  const pending = options.requestCompletion(context, 'short')
  useChatStore.setState({ activeThreadId: 'thread-b' })
  useChatStore.setState({ activeThreadId: 'thread-a' })
  resolve({ ok: true, completion: 'late' })
  expect(await pending).toBeNull()
})
test('revokes a displayed rich ghost synchronously so Tab cannot accept it in another task',async()=>{
  const editor = capture.editor!
  const key = writeRichInlineCompletionTestInternals.inlineCompletionKey
  await act(async()=>{editor.view.dispatch(editor.state.tr.setMeta(key,{
    text:'stale completion',action:{kind:'short',text:'stale completion'},anchor:editor.state.selection.head,feedback:{}
  }))})
  expect(container.querySelector('.write-rich-ghost-text')?.textContent).toContain('stale completion')
  await act(async()=>useChatStore.setState({activeThreadId:'thread-b'}))
  expect(key.getState(editor.state)).toBeNull()
  expect(container.querySelector('.write-rich-ghost-text')).toBeNull()
  await act(async()=>{editor.commands.keyboardShortcut('Tab')})
  expect(editor.getText()).not.toContain('stale completion')
})
