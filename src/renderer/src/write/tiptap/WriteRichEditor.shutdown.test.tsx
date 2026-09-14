// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import type { Editor as TiptapEditor } from '@tiptap/core'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { WriteRichEditor } from './WriteRichEditor'
import { createWriteShutdownHandler } from '../write-shutdown'
import { useWriteWorkspaceStore } from '../write-workspace-store'
const capture=vi.hoisted(()=>({editor:undefined as TiptapEditor|undefined}))
vi.mock('@tiptap/core',async importOriginal=>{
  const core=await importOriginal<typeof import('@tiptap/core')>()
  return {...core,Editor:class extends core.Editor{constructor(options:any){super(options);capture.editor=this}}}
})
let root:Root, container:HTMLDivElement, handler:ReturnType<typeof createWriteShutdownHandler>
const requestId='00000000-0000-4000-8000-000000000002'
beforeEach(async()=>{
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
