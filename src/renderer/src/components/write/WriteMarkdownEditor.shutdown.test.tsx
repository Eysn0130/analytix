// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { EditorView } from '@codemirror/view'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { WriteMarkdownEditor } from './WriteMarkdownEditor'
import { createWriteShutdownHandler } from '../../write/write-shutdown'
import { useWriteWorkspaceStore } from '../../write/write-workspace-store'
vi.mock('../../write/inline-completion',()=>({buildInlineCompletionExtension:()=>[],buildInlineCompletionPayload:()=>({})}))
vi.mock('../../write/markdown-live-preview',()=>({writeMarkdownLivePreviewExtensions:()=>[]}))
let root:Root, container:HTMLDivElement, handler:ReturnType<typeof createWriteShutdownHandler>
const requestId='00000000-0000-4000-8000-000000000001'
const view=()=>EditorView.findFromDOM(container.querySelector('.cm-content') as HTMLElement)!
beforeEach(async()=>{
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
  container.remove();useWriteWorkspaceStore.getState().resetWorkspace();vi.restoreAllMocks();vi.unstubAllGlobals()
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
