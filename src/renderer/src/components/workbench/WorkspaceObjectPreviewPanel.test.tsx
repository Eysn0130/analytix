// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useChatStore } from '../../store/chat-store'
import { useWorkspaceTabsStore, workspaceObjectTabId } from '../../store/workspace-tabs-store'
import { WorkspaceObjectPreviewPanel } from './WorkspaceObjectPreviewPanel'

const canvas = vi.hoisted(() => vi.fn((_props: Record<string, unknown>) => null)), file = vi.hoisted(() => vi.fn((_props: Record<string, unknown>) => null))
vi.mock('../../canvas/CanvasWorkspacePanel', () => ({ CanvasWorkspacePanel: canvas }))
vi.mock('../WorkspaceFilePreviewPanel', () => ({ WorkspaceFilePreviewPanel: file }))
let container: HTMLDivElement, root: Root
const initialChat = useChatStore.getState(), initialTabs = useWorkspaceTabsStore.getState()
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  canvas.mockClear(); file.mockClear()
  useChatStore.setState({ threads: [], activeThreadId: 'thread-a', workspaceRoot: '/project' })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => {
  await act(async () => root.unmount()); container.remove()
  useChatStore.setState(initialChat); useWorkspaceTabsStore.setState(initialTabs); vi.unstubAllGlobals()
})
async function show(path: string, explicit = false) {
  const id = workspaceObjectTabId('/project', path)
  useWorkspaceTabsStore.setState({ tabs: [{ id, kind: 'file', mode: 'file', title: path, path, workspaceRoot: '/project', ...(explicit ? { preview: 'canvas' as const } : {}) }], activeTabId: id, open: true, selectorOpen: false })
  await act(async () => root.render(createElement(WorkspaceObjectPreviewPanel, { workspaceRoot: '/project', target: { workspaceRoot: '/project', path }, onClose: () => {} })))
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
    await show('/project/image.png', true)
    expect(canvas).toHaveBeenCalled()
  })
})
