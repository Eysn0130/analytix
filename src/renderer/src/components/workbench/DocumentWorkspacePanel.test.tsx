// @vitest-environment jsdom
import { act, createElement, type ReactNode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { DocumentWorkspacePanel } from './DocumentWorkspacePanel'
import { useNativeOfficeStore } from '../../office/native-office-store'
import { useWriteWorkspaceStore } from '../../write/write-workspace-store'

vi.mock('react-i18next', async importOriginal => ({ ...await importOriginal<typeof import('react-i18next')>(), useTranslation: () => ({ t: (key:string) => key }) }))
vi.mock('../../office/NativeOfficePanel', () => ({ NativeOfficePanel: ({fileActions}: {fileActions:ReactNode}) => createElement('div',{'data-testid':'native-panel',role:'toolbar'},fileActions) }))
vi.mock('../write/WriteWorkspaceView', () => ({ WriteWorkspaceView: () => createElement('div',{'data-testid':'write-panel'}) }))
let root:Root, container:HTMLDivElement
const openEditorPath = vi.fn(async () => ({ok:true}))
const path = '/synthetic/report.docx'
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true)
  openEditorPath.mockClear()
  Object.defineProperty(window,'analytix',{configurable:true,value:{workspace:{openEditorPath}}})
  useNativeOfficeStore.setState({target:{workspace:'/synthetic',path},view:null,error:null})
  useWriteWorkspaceStore.setState({workspaceRoot:'/synthetic',activeFilePath:null})
  container=document.createElement('div');document.body.append(container);root=createRoot(container)
})
afterEach(async () => {
  await act(async () => root.unmount());container.remove()
  Reflect.deleteProperty(window,'analytix');vi.unstubAllGlobals()
  useNativeOfficeStore.setState({target:null,view:null,error:null})
})
async function render() {
  await act(async () => root.render(createElement(DocumentWorkspacePanel,{
    visible:true,input:'existing draft',setInput:vi.fn(),onSubmitPrompt:vi.fn(),focused:false,onToggleFocus:vi.fn(),onCollapse:vi.fn(),onOpenSettings:vi.fn(),onFocusConversation:vi.fn(),fileBrowser:createElement('div',{'data-testid':'file-browser'},'Files')
  })))
}
async function select(action:string) {
  const element=container.querySelector<HTMLSelectElement>('[aria-label="打开"]')!
  await act(async () => {element.value=action;element.dispatchEvent(new Event('change',{bubbles:true}))})
}
it('removes the duplicate native document header and dispatches actual file menu actions', async () => {
  await render()
  expect(container.querySelector('.document-workspace-header')).toBeNull()
  expect(container.querySelectorAll('[role="toolbar"]')).toHaveLength(1)
  await select('system')
  expect(openEditorPath).toHaveBeenLastCalledWith({workspaceRoot:'/synthetic',path,editorId:'system'})
  await select('file-manager')
  expect(openEditorPath).toHaveBeenLastCalledWith({workspaceRoot:'/synthetic',path,editorId:'file-manager'})
  expect(container.querySelector('[data-testid="file-browser"]')).toBeNull()
  await select('browse')
  expect(container.querySelector('[data-testid="file-browser"]')).not.toBeNull()
  expect(openEditorPath).toHaveBeenCalledTimes(2)
  await act(async () => container.querySelector('section')!.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true,cancelable:true})))
  expect(container.querySelector('[data-testid="file-browser"]')).toBeNull()
  expect(document.activeElement).toBe(container.querySelector('[aria-label="打开"]'))
})
it('preserves the existing nonnative Write header and file controls', async () => {
  useNativeOfficeStore.setState({target:null,view:null})
  useWriteWorkspaceStore.setState({activeFilePath:'/synthetic/notes.md'})
  await render()
  expect(container.querySelector('.document-workspace-header')).not.toBeNull()
  expect(container.querySelector('[data-testid="write-panel"]')).not.toBeNull()
  expect(container.querySelector('[aria-label="rightPanelFiles"]')).not.toBeNull()
})
