// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useChatStore } from '../../store/chat-store'
import { useWorkspaceTabsStore, workspaceObjectTabId } from '../../store/workspace-tabs-store'
import { useNativeOfficeStore } from '../../office/native-office-store'
import { WorkspaceObjectPreviewPanel } from './WorkspaceObjectPreviewPanel'

const canvas = vi.hoisted(() => vi.fn((_props: Record<string, unknown>) => null))
const file = vi.hoisted(() => vi.fn((_props: Record<string, unknown>) => null))
const native = vi.hoisted(() => vi.fn((_props: Record<string, unknown>) => null))
vi.mock('../../office/NativeOfficePanel',()=>({NativeOfficePanel:native}))
vi.mock('../../canvas/CanvasWorkspacePanel', () => ({ CanvasWorkspacePanel: canvas }))
vi.mock('../WorkspaceFilePreviewPanel', () => ({ WorkspaceFilePreviewPanel: file }))
let container: HTMLDivElement, root: Root
const initialChat = useChatStore.getState(), initialTabs = useWorkspaceTabsStore.getState()
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  canvas.mockClear(); file.mockClear(); native.mockClear()
  useNativeOfficeStore.setState({target:null,view:null,views:{},error:null})
  Object.defineProperty(window,'analytix',{configurable:true,value:{office:{request:vi.fn(async()=>({ok:true,view:null}))}}})
  useChatStore.setState({ threads: [], activeThreadId: 'thread-a', workspaceRoot: '/project' })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => {
  await act(async () => root.unmount()); container.remove()
  useChatStore.setState(initialChat); useWorkspaceTabsStore.setState(initialTabs); Reflect.deleteProperty(window,'analytix'); vi.unstubAllGlobals()
})
async function show(path: string, explicit = false) {
  const id = workspaceObjectTabId('/project', path)
  useWorkspaceTabsStore.setState({ tabs: [{ id, kind: 'file', mode: 'file', title: path, path, workspaceRoot: '/project', ...(explicit ? { preview: 'canvas' as const } : {}) }], activeTabId: id, open: true, selectorOpen: false })
  await act(async () => root.render(createElement(WorkspaceObjectPreviewPanel, { workspaceRoot: '/project', target: { workspaceRoot: '/project', path }, className: 'h-full max-h-full w-full', onClose: () => {} })))
}
describe('Canvas reuses the existing object file route', () => {
  it('routes .canvas using current thread and project without introducing a workspace mode', async () => {
    await show('/project/flow.canvas')
    expect(canvas).toHaveBeenCalled()
    expect(canvas.mock.calls.at(-1)?.[0]).toMatchObject({ threadId: 'thread-a', workspaceRoot: '/project', visible: true, activeTab: { kind: 'file', mode: 'file', path: '/project/flow.canvas' } })
    expect(file).not.toHaveBeenCalled()
  })
  it('preserves normal image and text previews unless Canvas was explicitly chosen', async () => {
    await show('/project/image.png')
    expect(file).toHaveBeenCalled(); expect(canvas).not.toHaveBeenCalled()
    expect(file.mock.calls.at(-1)?.[0]).toMatchObject({ className: 'h-full max-h-full w-full' })
    await show('/project/image.png', true)
    expect(canvas).toHaveBeenCalled()
  })
})


it('keeps a Present .canvas reference in the protected current-file preview', async () => {
  const protectedPresent = { threadId: 'thread-a', turnId: 'turn-a', callId: 'call_host_' + 'a'.repeat(64),
    resultItemId: 'item_result_' + 'b'.repeat(64), viewId: '11111111-1111-4111-8111-111111111111', snapshotDigest: 'c'.repeat(64), index: 1 }
  useWorkspaceTabsStore.setState({ tabs: [{ id: 'protected-present:' + protectedPresent.viewId + ':1', kind: 'file', mode: 'file',
    title: 'flow.canvas', preview: 'source', protectedPresent }], activeTabId: 'protected-present:' + protectedPresent.viewId + ':1', open: true, selectorOpen: false })
  const target = { path: 'flow.canvas', preview: 'source' as const, protectedPresent }
  await act(async () => root.render(createElement(WorkspaceObjectPreviewPanel, { workspaceRoot: '/project', target, onClose: () => {} })))
  expect(file).toHaveBeenCalled(); expect(canvas).not.toHaveBeenCalled()
  expect(file.mock.calls.at(-1)?.[0]).toMatchObject({ target })
})

it.each(['docx','pptx'])('opens the real protected-present %s tab with full selector identity',async kind=>{
  const protectedPresent = {threadId:'thread-a',turnId:'turn-a',callId:'call_host_'+'a'.repeat(64),resultItemId:'item_result_'+'b'.repeat(64),viewId:'11111111-1111-4111-8111-111111111111',snapshotDigest:'c'.repeat(64),index:0}
  const id='protected-present:'+protectedPresent.viewId+':0',target={path:'display.'+kind,preview:'source' as const,protectedPresent}
  useWorkspaceTabsStore.setState({tabs:[{id,kind:'file',mode:'file',title:target.path,preview:'source',protectedPresent}],activeTabId:id,open:true,selectorOpen:false})
  await act(async()=>root.render(createElement(WorkspaceObjectPreviewPanel,{workspaceRoot:'/project',target,onClose:()=>{}})))
  expect(native.mock.calls.at(-1)?.[0]).toMatchObject({visible:true,presentSelector:protectedPresent})
  expect(file).not.toHaveBeenCalled();expect(canvas).not.toHaveBeenCalled()
  const close=vi.spyOn(useNativeOfficeStore.getState(),'closePresent')
  await act(async()=>root.render(createElement(WorkspaceObjectPreviewPanel,{workspaceRoot:'/project',target:{...target,protectedPresent:{...protectedPresent}},onClose:()=>{}})))
  expect(close).not.toHaveBeenCalled();close.mockRestore()
})
it.each(['docx','pptx'])('keeps the full-height %s preview shell through loading and native readiness',async kind=>{
  const protectedPresent = {threadId:'thread-a',turnId:'turn-a',callId:'call_host_'+'a'.repeat(64),resultItemId:'item_result_'+'b'.repeat(64),viewId:'11111111-1111-4111-8111-111111111111',snapshotDigest:'c'.repeat(64),index:0}
  const id='protected-present:'+protectedPresent.viewId+':0',target={path:'display.'+kind,preview:'source' as const,protectedPresent}
  useWorkspaceTabsStore.setState({tabs:[{id,kind:'file',mode:'file',title:target.path,preview:'source',protectedPresent}],activeTabId:id,open:true,selectorOpen:false})
  const select=vi.spyOn(useNativeOfficeStore.getState(),'selectPresent').mockResolvedValue(true)
  try {
    await act(async()=>root.render(createElement(WorkspaceObjectPreviewPanel,{workspaceRoot:'/project',target,className:'max-h-full w-full caller-preview',onClose:()=>{}})))
    const shell=container.firstElementChild
    expect(shell).not.toBeNull()
    for (const name of ['h-full','min-h-0','flex','flex-col','max-h-full','w-full','caller-preview']) expect(shell!.classList.contains(name)).toBe(true)
    expect(shell!.querySelector('p[role="status"]')).not.toBeNull()
    expect(native).not.toHaveBeenCalled()
    await act(async()=>{useNativeOfficeStore.setState({target:{workspace:'/project',path:target.path,protectedPresent:{...protectedPresent}}})})
    expect(container.firstElementChild).toBe(shell)
    expect(shell!.querySelector('p[role="status"]')).toBeNull()
    expect(native.mock.calls.at(-1)?.[0]).toMatchObject({visible:true,presentSelector:protectedPresent})
    expect(file).not.toHaveBeenCalled();expect(canvas).not.toHaveBeenCalled()
  } finally { select.mockRestore() }
})
it('keeps protected spreadsheets on their existing reader and rejects a foreign Office tab owner',async()=>{
  const protectedPresent = {threadId:'thread-a',turnId:'turn-a',callId:'call_host_'+'a'.repeat(64),resultItemId:'item_result_'+'b'.repeat(64),viewId:'11111111-1111-4111-8111-111111111111',snapshotDigest:'c'.repeat(64),index:0}
  useWorkspaceTabsStore.setState({tabs:[],activeTabId:null,open:true,selectorOpen:false})
  await act(async()=>root.render(createElement(WorkspaceObjectPreviewPanel,{workspaceRoot:'/project',target:{path:'current.xlsx',protectedPresent},onClose:()=>{}})))
  expect(file).toHaveBeenCalled();expect(native).not.toHaveBeenCalled()
  await act(async()=>root.render(createElement(WorkspaceObjectPreviewPanel,{workspaceRoot:'/project',target:{path:'current.docx',protectedPresent},onClose:()=>{}})))
  expect(native).not.toHaveBeenCalled();expect(useNativeOfficeStore.getState().target).toBeNull()
})
